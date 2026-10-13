// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package egress

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/httpx"
)

// loopbackProxy starts a proxy that may reach loopback, resolves
// example.com to 127.0.0.1, uses the real dialer and permits one port.
func loopbackProxy(t *testing.T, port int) *harness {
	t.Helper()
	h := startProxy(t, func(o *Options) {
		o.Policy = httpx.Policy{AllowLoopback: true}
		o.Dialer = nil
		o.Ports = []int{port}
	})
	h.resolver.names["example.com"] = []netip.Addr{netip.MustParseAddr("127.0.0.1")}
	return h
}

// portOf returns the port of a test server's URL.
func portOf(t *testing.T, rawURL string) int {
	t.Helper()
	_, p, err := net.SplitHostPort(strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://"))
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Verifies: FN-013, FN-017.
// HTTPS to a destination goes through a CONNECT tunnel with TLS end to
// end: the destination's certificate is verified by the caller, and the
// proxy relays ciphertext.
func TestTransportReachesAnHTTPSUpstream_FN_017(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "hello from "+r.Host)
	}))
	defer upstream.Close()
	port := portOf(t, upstream.URL)
	h := loopbackProxy(t, port)

	tr := h.clientDialer().Transport([]string{"example.com"}, testFunction)
	defer tr.CloseIdleConnections()
	pool := x509.NewCertPool()
	pool.AddCert(upstream.Certificate())
	tr.TLSClientConfig.RootCAs = pool // httptest's certificate names example.com

	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	resp, err := client.Get("https://example.com:" + strconv.Itoa(port) + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "hello from example.com:"+strconv.Itoa(port) {
		t.Errorf("status %d, body %q", resp.StatusCode, body)
	}
	if got := h.counter(t, "plux_egress_tunnels_total", "result", resultOK); got != 1 {
		t.Errorf("ok = %v", got)
	}
	eventually(t, "the byte counters", func() bool {
		return h.counter(t, "plux_egress_bytes_total", "direction", directionFromUpstream) > 0 &&
			h.counter(t, "plux_egress_bytes_total", "direction", directionToUpstream) > 0
	})
}

// Verifies: FN-017.
// Plain HTTP is a tunnel as well; the proxy sees no HTTP beyond CONNECT.
func TestTransportTunnelsPlainHTTP_FN_017(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "plain "+r.URL.Path)
	}))
	defer upstream.Close()
	port := portOf(t, upstream.URL)
	h := loopbackProxy(t, port)

	tr := h.clientDialer().Transport([]string{"*.com"}, testFunction)
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	resp, err := client.Get("http://example.com:" + strconv.Itoa(port) + "/path")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "plain /path" {
		t.Errorf("body %q", body)
	}
}

// Verifies: FN-017.
func TestTransportRefusalIsErrRefused_FN_017(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	port := portOf(t, upstream.URL)
	h := loopbackProxy(t, port)

	tr := h.clientDialer().Transport([]string{"other.example.org"}, testFunction)
	defer tr.CloseIdleConnections()
	_, err := (&http.Client{Transport: tr, Timeout: 10 * time.Second}).Get("http://example.com:" + strconv.Itoa(port) + "/")
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "PLX-7010") {
		t.Fatalf("err = %v, want ErrRefused carrying PLX-7010", err)
	}
	if got := h.counter(t, "plux_egress_tunnels_total", "result", resultRefusedDomain); got != 1 {
		t.Errorf("refused_domain = %v", got)
	}
}

// Verifies: FN-017.
func TestDialerMapsProxyAnswers_FN_017(t *testing.T) {
	t.Parallel()
	h := startProxy(t, func(o *Options) { o.MaxTunnels = 1 })
	h.resolver.names["api.example.com"] = []netip.Addr{netip.MustParseAddr("8.8.8.8")}
	h.resolver.names["down.example.com"] = []netip.Addr{netip.MustParseAddr("8.8.4.4")}
	h.dialer.failFor = map[string]bool{"8.8.4.4:443": true}
	d := h.clientDialer()
	ctx := context.Background()
	allow := []string{"*.example.com"}

	if _, err := d.DialContext(ctx, "tcp", "evil.org:443", allow, testFunction); !errors.Is(err, ErrRefused) {
		t.Errorf("domain: %v", err)
	}
	if _, err := d.DialContext(ctx, "tcp", "127.0.0.1:443", allow, testFunction); !errors.Is(err, ErrRefused) {
		t.Errorf("literal: %v", err)
	}
	if _, err := d.DialContext(ctx, "tcp", "down.example.com:443", allow, testFunction); !errors.Is(err, ErrUpstream) {
		t.Errorf("unreachable: %v", err)
	}
	first, err := d.DialContext(ctx, "tcp", "api.example.com:443", allow, testFunction)
	if err != nil {
		t.Fatalf("first tunnel: %v", err)
	}
	defer func() { _ = first.Close() }()
	if _, err := d.DialContext(ctx, "tcp", "api.example.com:443", allow, testFunction); !errors.Is(err, ErrLimited) {
		t.Errorf("cap: %v", err)
	}
}

// Verifies: FN-017.
func TestDialerKeepsBytesTheDestinationSendsFirst_FN_017(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		_, _ = c.Write([]byte("220 hello\r\n"))
		_, _ = io.Copy(io.Discard, c)
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	h := loopbackProxy(t, port)

	conn, err := h.clientDialer().DialContext(context.Background(), "tcp", "example.com:"+strconv.Itoa(port), []string{"example.com"}, testFunction)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 11)
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "220 hello\r\n" {
		t.Errorf("read %q, %v", buf, err)
	}
}

// Verifies: FN-013.
func TestDialerRejectsAnUntrustedProxy_FN_013(t *testing.T) {
	t.Parallel()
	h := startProxy(t, nil)
	d := h.clientDialer()
	d.TLS = newPKI(t).clientConfig(nil) // trusts another CA, presents nothing
	if _, err := d.DialContext(context.Background(), "tcp", "api.example.com:443", []string{"api.example.com"}, testFunction); err == nil {
		t.Error("dialled a proxy whose certificate the caller does not trust")
	}
}

// Verifies: FN-017.
func TestDialerValidatesItsArguments_FN_017(t *testing.T) {
	t.Parallel()
	d := &Dialer{Addr: "127.0.0.1:1"}
	ctx := context.Background()
	cases := map[string]func() error{
		"network": func() error {
			_, err := d.DialContext(ctx, "udp", "a.test:443", []string{"a.test"}, testFunction)
			return err
		},
		"empty allowlist": func() error {
			_, err := d.DialContext(ctx, "tcp", "a.test:443", nil, testFunction)
			return err
		},
		"line break in the allowlist": func() error {
			_, err := d.DialContext(ctx, "tcp", "a.test:443", []string{"a.test\r\nX: y"}, testFunction)
			return err
		},
		"line break in the function": func() error {
			_, err := d.DialContext(ctx, "tcp", "a.test:443", []string{"a.test"}, "a/b/c@1\r\nX: y")
			return err
		},
	}
	for name, run := range cases {
		if err := run(); err == nil || strings.Contains(err.Error(), "connect to the proxy") {
			t.Errorf("%s: err = %v, want an argument error before any connection", name, err)
		}
	}
}

// Verifies: FN-017.
func TestTransportSettings_FN_017(t *testing.T) {
	t.Parallel()
	tr := (&Dialer{Addr: "127.0.0.1:1"}).Transport([]string{"a.test"}, testFunction)
	if tr.Proxy != nil || tr.ForceAttemptHTTP2 || tr.MaxResponseHeaderBytes <= 0 || tr.ResponseHeaderTimeout <= 0 || tr.DialContext == nil {
		t.Errorf("transport = proxy %v, h2 %v, header bytes %d, header timeout %v",
			tr.Proxy != nil, tr.ForceAttemptHTTP2, tr.MaxResponseHeaderBytes, tr.ResponseHeaderTimeout)
	}
}
