// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gitsync

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/nightCode42/plux3/backend/internal/httpx"
)

// Resolver looks a host name up; *net.Resolver's LookupNetIP is one.
type Resolver func(ctx context.Context, host string) ([]netip.Addr, error)

// systemResolver resolves with the system resolver.
func systemResolver(ctx context.Context, host string) ([]netip.Addr, error) {
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("gitsync: resolve %s: %w", host, err)
	}
	return addrs, nil
}

// errRefusedAddress is reported when a name resolves to an address the
// policy forbids.
var errRefusedAddress = errors.New("gitsync: the remote's address is not allowed")

// proxy is a loopback CONNECT proxy for the connections of one push. It
// is the only way go-git's transport can be given an address check that
// holds for redirects and for every DNS answer. It owns its listener and
// every goroutine it starts; Close ends them all.
type proxy struct {
	policy  httpx.Policy
	resolve Resolver
	auth    string
	ln      net.Listener
	srv     *http.Server

	wg     sync.WaitGroup
	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	closed bool
}

// startProxy listens on a loopback port. secret authenticates the one
// client that knows it, so another process on the host cannot use the proxy.
func startProxy(policy httpx.Policy, resolve Resolver, secret string) (*proxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("gitsync: listen for the proxy: %w", err)
	}
	p := &proxy{
		policy: policy, resolve: resolve, ln: ln, conns: map[net.Conn]struct{}{},
		auth: "Basic " + base64.StdEncoding.EncodeToString([]byte("plux:"+secret)),
	}
	p.srv = &http.Server{Handler: http.HandlerFunc(p.serve), ReadHeaderTimeout: 10 * time.Second}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		_ = p.srv.Serve(ln) // ends with ErrServerClosed when Close runs
	}()
	return p, nil
}

// address is the proxy's address, for the transport.
func (p *proxy) address() string { return p.ln.Addr().String() }

// Close stops accepting, closes every tunnel and waits for the goroutines.
func (p *proxy) Close() {
	p.mu.Lock()
	p.closed = true
	for c := range p.conns {
		_ = c.Close()
	}
	p.mu.Unlock()
	_ = p.srv.Close()
	p.wg.Wait()
}

// serve answers a CONNECT request and refuses everything else.
func (p *proxy) serve(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Proxy-Authorization")), []byte(p.auth)) != 1 {
		http.Error(w, "proxy authentication required", http.StatusProxyAuthRequired)
		return
	}
	if r.Method != http.MethodConnect {
		http.Error(w, "only CONNECT is served", http.StatusMethodNotAllowed)
		return
	}
	upstream, err := p.dial(r.Context(), r.Host)
	if err != nil {
		http.Error(w, "the destination is not reachable", http.StatusForbidden)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		_ = upstream.Close()
		http.Error(w, "cannot hijack", http.StatusInternalServerError)
		return
	}
	client, _, err := hj.Hijack()
	if err != nil {
		_ = upstream.Close()
		return
	}
	if !p.track(client, upstream) {
		_ = client.Close()
		_ = upstream.Close()
		return
	}
	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		p.untrack(client, upstream)
		return
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		done := make(chan struct{}, 2)
		go func() { _, _ = io.Copy(upstream, client); done <- struct{}{} }()
		go func() { _, _ = io.Copy(client, upstream); done <- struct{}{} }()
		<-done // either side ending ends the tunnel
		p.untrack(client, upstream)
		<-done
	}()
}

// track registers a tunnel's connections, or reports that the proxy closed.
func (p *proxy) track(conns ...net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	for _, c := range conns {
		p.conns[c] = struct{}{}
	}
	return true
}

// untrack closes a tunnel's connections.
func (p *proxy) untrack(conns ...net.Conn) {
	p.mu.Lock()
	for _, c := range conns {
		delete(p.conns, c)
	}
	p.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

// dial resolves the target, applies the policy to every address the name
// has, and connects to the address it checked.
func (p *proxy) dial(ctx context.Context, target string) (net.Conn, error) {
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return nil, fmt.Errorf("gitsync: %w", errRefusedAddress)
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("gitsync: %w", errRefusedAddress)
	}
	addrs, err := allowedAddresses(ctx, p.policy, p.resolve, host)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	var last error
	for _, a := range addrs {
		c, err := d.DialContext(ctx, "tcp", netip.AddrPortFrom(a, uint16(port)).String())
		if err == nil {
			return c, nil
		}
		last = err
	}
	return nil, fmt.Errorf("gitsync: connect to the remote: %w", last)
}

// allowedAddresses returns the addresses of a host, all of which the policy allows.
// One forbidden address refuses the name: a name that answers with a mix is
// how rebinding attacks are written.
func allowedAddresses(ctx context.Context, policy httpx.Policy, resolve Resolver, host string) ([]netip.Addr, error) {
	addrs, err := resolveAll(ctx, resolve, host)
	if err != nil {
		return nil, err
	}
	for _, a := range addrs {
		if err := policy.Check(a); err != nil {
			return nil, fmt.Errorf("gitsync: %w: %w", errRefusedAddress, err)
		}
	}
	return addrs, nil
}

// resolveAll resolves a host, which may be an address literal.
func resolveAll(ctx context.Context, resolve Resolver, host string) ([]netip.Addr, error) {
	if a, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{a}, nil
	}
	addrs, err := resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("gitsync: %s has no address", host)
	}
	return addrs, nil
}
