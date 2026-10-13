// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package egress

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

var (
	// ErrRefused is returned when the proxy refuses a tunnel: the domain,
	// port or address is not allowed (PLX-7010). The error carries the
	// code and the proxy's short reason, never upstream detail.
	ErrRefused = errors.New("egress: the destination is not allowed")
	// ErrUpstream is returned when the proxy could not reach the
	// destination.
	ErrUpstream = errors.New("egress: the destination is unreachable")
	// ErrLimited is returned when the proxy is at its tunnel cap or is
	// shutting down.
	ErrLimited = errors.New("egress: the proxy is at capacity")
)

// maxReplyBytes bounds how much of a refusal body the client reads.
const maxReplyBytes = 512

// Dialer opens tunnels through an egress proxy. It is the only way
// fnrunner connects outward. It is safe for concurrent use.
type Dialer struct {
	// Addr is the proxy's host:port.
	Addr string
	// TLS is this role's mutual-TLS identity and the internal CA; its
	// ServerName is derived from Addr when empty. It is cloned, and TLS 1.3
	// is the minimum.
	TLS *tls.Config
	// Timeout bounds the connection to the proxy and its answer; zero means
	// ten seconds.
	Timeout time.Duration
}

// DialContext opens a tunnel to addr ("host:port") on behalf of the
// function, which is allowed to reach the domains in allow. network must
// be a TCP network. The returned connection is the tunnel: bytes written
// reach the destination unchanged.
func (d *Dialer) DialContext(ctx context.Context, network, addr string, allow []string, function string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("egress.Dial: network %q is not supported", network)
	}
	if len(allow) == 0 {
		return nil, errors.New("egress.Dial: the allowlist is empty")
	}
	allowList := strings.Join(allow, ",")
	if strings.ContainsAny(allowList+function+addr, "\r\n") {
		return nil, errors.New("egress.Dial: a header value contains a line break")
	}
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	conf := d.TLS.Clone()
	if conf == nil {
		conf = new(tls.Config)
	}
	conf.MinVersion = tls.VersionTLS13
	conf.NextProtos = []string{"http/1.1"}
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: timeout}, Config: conf}
	conn, err := dialer.DialContext(ctx, "tcp", d.Addr)
	if err != nil {
		return nil, fmt.Errorf("egress.Dial: connect to the proxy: %w", err)
	}
	tunnel, err := handshake(ctx, conn, timeout, addr, allowList, function)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return tunnel, nil
}

// handshake sends CONNECT and reads the answer. A deadline and the
// context's cancellation both interrupt it.
func handshake(ctx context.Context, conn net.Conn, timeout time.Duration, addr, allow, function string) (net.Conn, error) {
	_ = conn.SetDeadline(time.Now().Add(timeout))
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()

	req := "CONNECT " + addr + " HTTP/1.1\r\nHost: " + addr + "\r\n" +
		HeaderAllow + ": " + allow + "\r\n" +
		HeaderFunction + ": " + function + "\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		return nil, fmt.Errorf("egress.Dial: send CONNECT: %w", err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return nil, fmt.Errorf("egress.Dial: read the answer: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, replyError(resp)
	}
	if !stop() {
		return nil, fmt.Errorf("egress.Dial: %w", ctx.Err())
	}
	_ = conn.SetDeadline(time.Time{})
	// The destination may speak first; keep what has been read.
	return &bufferedConn{Conn: conn, r: br}, nil
}

// replyError maps a refusal to a sentinel error.
func replyError(resp *http.Response) error {
	reason, _ := io.ReadAll(io.LimitReader(resp.Body, maxReplyBytes))
	text := strings.TrimSpace(string(reason))
	switch resp.StatusCode {
	case http.StatusForbidden:
		return fmt.Errorf("%w (%s): %s", ErrRefused, resp.Header.Get(HeaderError), text)
	case http.StatusBadGateway:
		return fmt.Errorf("%w: %s", ErrUpstream, text)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("%w: %s", ErrLimited, text)
	}
	return fmt.Errorf("egress.Dial: the proxy answered %d: %s", resp.StatusCode, text)
}

// Transport returns an HTTP transport whose every connection is a tunnel
// through the proxy for one function. Plain HTTP and HTTPS alike are
// tunnelled; for HTTPS the TLS handshake with the destination happens
// inside the tunnel, with the system roots, so the proxy never sees
// plaintext. It uses no proxy from the environment and speaks HTTP/1.1.
// Build one per function invocation or per function: it pools
// connections. The transport follows no redirect by itself; the HTTP
// client around it must refuse them.
func (d *Dialer) Transport(allow []string, function string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return d.DialContext(ctx, network, addr, allow, function)
		},
		Proxy:                  nil,
		ForceAttemptHTTP2:      false,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  30 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		MaxIdleConns:           16,
		MaxIdleConnsPerHost:    4,
		IdleConnTimeout:        30 * time.Second,
		ExpectContinueTimeout:  time.Second,
	}
}
