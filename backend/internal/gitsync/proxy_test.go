// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gitsync

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/httpx"
)

// connect sends one request to the proxy and returns the status line code.
func connect(t *testing.T, p *proxy, method, target, auth string) int {
	t.Helper()
	conn, err := net.Dial("tcp", p.address())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	req := fmt.Sprintf("%s %s HTTP/1.1\r\nHost: example.com\r\n", method, target)
	if auth != "" {
		req += "Proxy-Authorization: " + auth + "\r\n"
	}
	if _, err := conn.Write([]byte(req + "\r\n")); err != nil {
		t.Fatal(err)
	}
	res, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode
}

// Verifies: GOV-011, SEC-163.
// The proxy that carries a push refuses reserved addresses whether they are
// written as literals or come from a name, refuses a mixed answer, requires
// its secret, serves CONNECT only, and tunnels to an allowed address.
func TestProxy_AddressPolicy_GOV_011(t *testing.T) {
	t.Parallel()
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = echo.Close() })
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = c.Write([]byte("hello")) }()
		}
	}()
	resolver := func(_ context.Context, host string) ([]netip.Addr, error) {
		switch host {
		case "internal.example":
			return []netip.Addr{netip.MustParseAddr("10.0.0.5")}, nil
		case "mixed.example":
			return []netip.Addr{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("169.254.169.254")}, nil
		case "local.example":
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}
		return nil, fmt.Errorf("no such host %s", host)
	}
	strict, err := startProxy(httpx.Policy{}, resolver, "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer strict.Close()
	good := "Basic " + base64.StdEncoding.EncodeToString([]byte("plux:secret"))
	port := echo.Addr().(*net.TCPAddr).Port
	for _, target := range []string{
		fmt.Sprintf("127.0.0.1:%d", port), "169.254.169.254:443", "[::1]:443", "10.0.0.1:443",
		"internal.example:443", "mixed.example:443", "local.example:443", "unknown.example:443", "no-port",
	} {
		if got := connect(t, strict, http.MethodConnect, target, good); got != http.StatusForbidden {
			t.Errorf("CONNECT %s = %d, want 403", target, got)
		}
	}
	if got := connect(t, strict, http.MethodConnect, "example.com:443", ""); got != http.StatusProxyAuthRequired {
		t.Errorf("no secret = %d", got)
	}
	if got := connect(t, strict, http.MethodConnect, "example.com:443", "Basic "+base64.StdEncoding.EncodeToString([]byte("plux:wrong"))); got != http.StatusProxyAuthRequired {
		t.Errorf("wrong secret = %d", got)
	}
	if got := connect(t, strict, http.MethodGet, "/", good); got != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", got)
	}

	open, err := startProxy(httpx.Policy{AllowLoopback: true}, resolver, "secret")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", open.address())
	if err != nil {
		t.Fatal(err)
	}
	target := fmt.Sprintf("127.0.0.1:%d", port)
	if _, err := fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: %s\r\n\r\n", target, target, good); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(conn)
	if res, err := http.ReadResponse(r, nil); err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT to an allowed address: %v, %v", res, err)
	}
	buf := make([]byte, 5)
	if _, err := r.Read(buf); err != nil || !strings.HasPrefix(string(buf), "hello") {
		t.Errorf("tunnel read = %q, %v", buf, err)
	}
	_ = conn.Close()
	open.Close() // returns only when every goroutine of the proxy has ended
}
