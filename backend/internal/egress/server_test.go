// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package egress

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/httpx"
)

const testFunction = "shop/prod/checkout@3"

// Verifies: FN-013, FN-017.
func TestConnectDecisions_FN_017(t *testing.T) {
	t.Parallel()
	h := startProxy(t, nil)
	h.resolver.names = map[string][]netip.Addr{
		"private.example.com":   addrs(t, "10.0.0.5"),
		"loopback.example.com":  addrs(t, "127.0.0.1"),
		"metadata.example.com":  addrs(t, "169.254.169.254"),
		"ula.example.com":       addrs(t, "fd00::1"),
		"linklocal.example.com": addrs(t, "fe80::1"),
		"mapped.example.com":    addrs(t, "::ffff:10.0.0.1"),
		"mapped-lo.example.com": addrs(t, "::ffff:127.0.0.1"),
		"rfc1918.example.com":   addrs(t, "192.168.1.1"),
		"cgnat.example.com":     addrs(t, "100.64.0.1"),
		"mixed.example.com":     addrs(t, "8.8.8.8", "10.0.0.1"),
		"mixed-rev.example.com": addrs(t, "10.0.0.1", "8.8.8.8"),
	}
	cases := []struct {
		name   string
		target string
		allow  string
		status int
		result string
	}{
		{"IPv4 literal", "127.0.0.1:443", "*.example.com", 403, resultRefusedIPLit},
		{"IPv4 literal on the allowlist", "8.8.8.8:443", "*.example.com,8.8.8.8", 403, resultRefusedIPLit},
		{"metadata literal", "169.254.169.254:80", "*.example.com", 403, resultRefusedIPLit},
		{"IPv6 literal", "[::1]:443", "*.example.com", 403, resultRefusedIPLit},
		{"IPv6 zone literal", "[fe80::1%25eth0]:443", "*.example.com", 403, resultRefusedIPLit},
		{"decimal literal", "2130706433:443", "*.example.com", 403, resultRefusedIPLit},
		{"hexadecimal literal", "0x7f.1:443", "*.example.com", 403, resultRefusedIPLit},
		{"port outside the set", "api.example.com:8080", "*.example.com", 403, resultRefusedPort},
		{"port is checked first", "127.0.0.1:22", "*.example.com", 403, resultRefusedPort},
		{"domain not on the allowlist", "evil.org:443", "*.example.com", 403, resultRefusedDomain},
		{"apex is not a wildcard match", "example.com:443", "*.example.com", 403, resultRefusedDomain},
		{"private address", "private.example.com:443", "*.example.com", 403, resultRefusedAddress},
		{"loopback address", "loopback.example.com:443", "*.example.com", 403, resultRefusedAddress},
		{"metadata address", "metadata.example.com:443", "*.example.com", 403, resultRefusedAddress},
		{"IPv6 unique local", "ula.example.com:443", "*.example.com", 403, resultRefusedAddress},
		{"IPv6 link-local", "linklocal.example.com:443", "*.example.com", 403, resultRefusedAddress},
		{"IPv4-mapped private", "mapped.example.com:443", "*.example.com", 403, resultRefusedAddress},
		{"IPv4-mapped loopback", "mapped-lo.example.com:443", "*.example.com", 403, resultRefusedAddress},
		{"RFC 1918", "rfc1918.example.com:443", "*.example.com", 403, resultRefusedAddress},
		{"carrier-grade NAT", "cgnat.example.com:443", "*.example.com", 403, resultRefusedAddress},
		{"public first, private second", "mixed.example.com:443", "*.example.com", 403, resultRefusedAddress},
		{"private first, public second", "mixed-rev.example.com:443", "*.example.com", 403, resultRefusedAddress},
		{"unresolvable", "nowhere.example.com:443", "*.example.com", 502, resultUpstreamFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := h.counter(t, "plux_egress_tunnels_total", "result", tc.result)
			resp, _, _ := h.rawConnect(t, connectRequest(tc.target, tc.allow, testFunction))
			if resp.StatusCode != tc.status {
				t.Fatalf("status %d, want %d", resp.StatusCode, tc.status)
			}
			if tc.status == 403 && resp.Header.Get(HeaderError) != "PLX-7010" {
				t.Errorf("Plux-Error = %q", resp.Header.Get(HeaderError))
			}
			if got := h.counter(t, "plux_egress_tunnels_total", "result", tc.result); got != before+1 {
				t.Errorf("%s = %v, want %v", tc.result, got, before+1)
			}
		})
	}
	if got := h.dialer.addresses(); len(got) != 0 {
		t.Errorf("a refused request was dialled: %v", got)
	}
}

// Verifies: FN-017.
// The proxy dials the address it validated and never asks the resolver
// again, so a name that changes its answer cannot steer the connection
// (DNS rebinding).
func TestDialsTheResolvedAddress_FN_017(t *testing.T) {
	t.Parallel()
	h := startProxy(t, nil)
	h.resolver.names["api.example.com"] = addrs(t, "93.184.216.34", "2606:4700::1111")
	h.dialer.failFor = map[string]bool{"93.184.216.34:443": true}

	resp, conn, _ := h.rawConnect(t, connectRequest("API.Example.com.:443", "*.example.com", testFunction))
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	defer func() { _ = conn.Close() }()

	if got, want := h.dialer.addresses(), []string{"93.184.216.34:443", "[2606:4700::1111]:443"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("dialled %v, want %v", got, want)
	}
	if got := h.resolver.lookups(); len(got) != 1 || got[0] != "api.example.com" {
		t.Errorf("lookups = %v, want one for the normalised name", got)
	}
}

// Verifies: FN-017.
func TestIPv4MappedUpstreamIsDialledUnmapped_FN_017(t *testing.T) {
	t.Parallel()
	h := startProxy(t, nil)
	h.resolver.names["api.example.com"] = addrs(t, "::ffff:8.8.8.8")
	resp, _, _ := h.rawConnect(t, connectRequest("api.example.com:80", "api.example.com", testFunction))
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if got := h.dialer.addresses(); len(got) != 1 || got[0] != "8.8.8.8:80" {
		t.Errorf("dialled %v", got)
	}
}

// Verifies: FN-017.
func TestUpstreamDialFailure_FN_017(t *testing.T) {
	t.Parallel()
	h := startProxy(t, nil)
	h.resolver.names["api.example.com"] = addrs(t, "8.8.8.8")
	h.dialer.failFor = map[string]bool{"8.8.8.8:443": true}
	resp, _, _ := h.rawConnect(t, connectRequest("api.example.com:443", "api.example.com", testFunction))
	if resp.StatusCode != 502 {
		t.Fatalf("status %d, want 502", resp.StatusCode)
	}
	if got := h.counter(t, "plux_egress_tunnels_total", "result", resultUpstreamFailed); got != 1 {
		t.Errorf("upstream_failed = %v", got)
	}
}

// Verifies: FN-017.
func TestLoopbackIsReachableOnlyWhenConfigured_FN_017(t *testing.T) {
	t.Parallel()
	h := startProxy(t, func(o *Options) { o.Policy = httpx.Policy{AllowLoopback: true} })
	h.resolver.names["local.example.com"] = addrs(t, "127.0.0.1")
	h.resolver.names["private.example.com"] = addrs(t, "10.0.0.1")
	if resp, _, _ := h.rawConnect(t, connectRequest("local.example.com:443", "*.example.com", testFunction)); resp.StatusCode != 200 {
		t.Errorf("loopback with AllowLoopback: status %d", resp.StatusCode)
	}
	if resp, _, _ := h.rawConnect(t, connectRequest("private.example.com:443", "*.example.com", testFunction)); resp.StatusCode != 403 {
		t.Errorf("private with AllowLoopback: status %d", resp.StatusCode)
	}
}

// Verifies: FN-017.
func TestNonConnectIsRefused_FN_017(t *testing.T) {
	t.Parallel()
	h := startProxy(t, nil)
	for _, req := range []string{
		"GET / HTTP/1.1\r\nHost: api.example.com\r\n\r\n",
		"GET http://api.example.com/ HTTP/1.1\r\nHost: api.example.com\r\n\r\n",
		"POST / HTTP/1.1\r\nHost: api.example.com\r\nContent-Length: 0\r\n\r\n",
	} {
		resp, _, _ := h.rawConnect(t, req)
		if resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Allow") != "CONNECT" {
			t.Errorf("%q: status %d, Allow %q", req[:12], resp.StatusCode, resp.Header.Get("Allow"))
		}
	}
	if got := h.counter(t, "plux_egress_tunnels_total", "result", resultBadRequest); got != 3 {
		t.Errorf("bad_request = %v", got)
	}
}

// Verifies: FN-017.
func TestMalformedConnectIsABadRequest_FN_017(t *testing.T) {
	t.Parallel()
	h := startProxy(t, nil)
	cases := map[string]string{
		"no allowlist":         connectRequest("api.example.com:443", "", testFunction),
		"empty allowlist":      connectRequest("api.example.com:443", " , ", testFunction),
		"only invalid entries": connectRequest("api.example.com:443", "10.0.0.1, *", testFunction),
		"no function":          connectRequest("api.example.com:443", "*.example.com", ""),
		"function malformed":   connectRequest("api.example.com:443", "*.example.com", "just-a-name"),
		"function with spaces": connectRequest("api.example.com:443", "*.example.com", "a/b/c d@1"),
		"no port":              connectRequest("api.example.com", "*.example.com", testFunction),
		"port zero":            connectRequest("api.example.com:0", "*.example.com", testFunction),
		"port too large":       connectRequest("api.example.com:70000", "*.example.com", testFunction),
		"bad host name":        connectRequest("a$b.example.com:443", "*.example.com", testFunction),
		"non-ASCII host":       connectRequest("bücher.example:443", "*.example", testFunction),
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			resp, _, _ := h.rawConnect(t, req)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status %d, want 400", resp.StatusCode)
			}
		})
	}
	if got := h.dialer.addresses(); len(got) != 0 {
		t.Errorf("dialled %v", got)
	}
}

// Verifies: SEC-043, FN-013.
func TestClientCertificateIsRequired_FN_013(t *testing.T) {
	t.Parallel()
	h := startProxy(t, nil)
	other := newPKI(t)
	foreign, _, _ := other.issue(t, t.TempDir(), "intruder")

	for name, conf := range map[string]*tls.Config{
		"no certificate": h.pki.clientConfig(nil),
		"certificate of another CA": func() *tls.Config {
			c := h.pki.clientConfig(&foreign)
			return c
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			d := tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: conf}
			conn, err := d.DialContext(context.Background(), "tcp", h.addr)
			if err != nil {
				return // refused during the handshake
			}
			defer func() { _ = conn.Close() }()
			// TLS 1.3 completes the client's handshake before the server
			// has judged the certificate; the refusal arrives on first use.
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			_, _ = conn.Write([]byte(connectRequest("api.example.com:443", "*.example.com", testFunction)))
			if n, err := conn.Read(make([]byte, 1)); err == nil || n != 0 {
				t.Errorf("read after a refused handshake: %d bytes, %v", n, err)
			}
		})
	}
	if got := h.resolver.lookups(); len(got) != 0 {
		t.Errorf("an unauthenticated peer caused lookups: %v", got)
	}
}

// Verifies: FN-017.
func TestTunnelCap_FN_017(t *testing.T) {
	t.Parallel()
	h := startProxy(t, func(o *Options) { o.MaxTunnels = 1 })
	h.resolver.names["api.example.com"] = addrs(t, "8.8.8.8")
	req := connectRequest("api.example.com:443", "api.example.com", testFunction)

	first, conn, _ := h.rawConnect(t, req)
	if first.StatusCode != 200 {
		t.Fatalf("first status %d", first.StatusCode)
	}
	second, _, _ := h.rawConnect(t, req)
	if second.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("second status %d, want 503", second.StatusCode)
	}
	if got := h.counter(t, "plux_egress_tunnels_total", "result", resultLimited); got != 1 {
		t.Errorf("limited = %v", got)
	}

	_ = conn.Close()
	eventually(t, "the slot to be released", func() bool {
		resp, c, _ := h.rawConnect(t, req)
		defer func() { _ = c.Close() }()
		return resp.StatusCode == 200
	})
}

// Verifies: FN-017.
func TestRefusalIsLoggedWithoutPayload_FN_017(t *testing.T) {
	t.Parallel()
	h := startProxy(t, nil)
	req := "CONNECT evil.org:443 HTTP/1.1\r\nHost: evil.org:443\r\n" +
		HeaderAllow + ": *.example.com\r\n" + HeaderFunction + ": " + testFunction + "\r\n" +
		"Authorization: Bearer s3cret-token\r\nCookie: session=abc\r\n\r\n"
	resp, _, _ := h.rawConnect(t, req)
	body := resp.Body
	if resp.StatusCode != 403 || strings.Contains(string(body), "evil.org") {
		t.Fatalf("status %d, body %q: the answer must not echo the destination", resp.StatusCode, body)
	}
	eventually(t, "the refusal to be logged", func() bool { return strings.Contains(h.logs.String(), "egress refused") })
	logged := h.logs.String()
	for _, want := range []string{testFunction, "evil.org", resultRefusedDomain} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log lacks %q: %s", want, logged)
		}
	}
	for _, leak := range []string{"s3cret-token", "session=abc", "Authorization"} {
		if strings.Contains(logged, leak) {
			t.Errorf("the log leaks %q: %s", leak, logged)
		}
	}
}

// Verifies: FN-013.
func TestNewRequiresMutualTLS_FN_013(t *testing.T) {
	t.Parallel()
	if _, err := New(Options{}); err == nil {
		t.Error("New without TLS succeeded")
	}
	if _, err := New(Options{TLS: &tls.Config{MinVersion: tls.VersionTLS13}}); err == nil {
		t.Error("New without a required client certificate succeeded")
	}
}

// Verifies: FN-017.
func TestStopClosesOpenTunnels_FN_017(t *testing.T) {
	t.Parallel()
	h := startProxy(t, nil)
	h.resolver.names["api.example.com"] = addrs(t, "8.8.8.8")
	resp, conn, br := h.rawConnect(t, connectRequest("api.example.com:443", "api.example.com", testFunction))
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // no grace: open tunnels are closed at once
	if err := h.srv.Stop(ctx); err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("Stop: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := br.ReadByte(); err == nil {
		t.Error("the tunnel survived Stop")
	}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.srv.Serve(ln); err == nil {
		t.Error("Serve after Stop succeeded")
	}
}
