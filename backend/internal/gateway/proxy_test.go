// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// front serves the gateway's streaming route for the fixture's device:
// everything after /gw/ is the target on the "bank" upstream with no
// operations.
func (f *fixture) front() *httptest.Server {
	f.t.Helper()
	u := f.ups["bank"]
	u.Operations = nil
	u.Methods = []string{"GET", "POST"}
	f.ups["bank"] = u
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := Request{Upstream: "bank", Method: r.Method, Target: strings.TrimPrefix(r.URL.RequestURI(), "/gw"), Header: r.Header}
		fail := func(w http.ResponseWriter, err error) {
			w.WriteHeader(map[plxerr.Code]int{plxerr.GatewayUpstreamFailed: 502, plxerr.GatewayDestinationRefused: 403}[code(err)])
		}
		p, err := f.gw.Begin(r.Context(), f.dev, req)
		if err != nil {
			fail(w, err)
			return
		}
		plan, err := p.Finish(r.Context(), nil)
		if err != nil {
			fail(w, err)
			return
		}
		f.gw.Proxy(w, r, plan, fail)
	}))
	f.t.Cleanup(srv.Close)
	return srv
}

// Verifies: SEC-030, SEC-031.
// A response streams through the gateway as the upstream writes it, so
// that server-sent events arrive when they are sent and no body is
// buffered whole.
func TestProxyStreamsServerSentEvents(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	f.handler = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Set-Cookie", "upstream=1")
		_, _ = io.WriteString(w, "data: one\n\n")
		w.(http.Flusher).Flush()
		<-release
		_, _ = io.WriteString(w, "data: two\n\n")
	}
	srv := f.front()
	resp, err := http.Get(srv.URL + "/gw/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil || line != "data: one\n" {
		t.Fatalf("the first event = %q, %v: it must arrive before the upstream finishes", line, err)
	}
	if resp.Header.Get("Set-Cookie") != "" {
		t.Error("the upstream's cookie reached the device")
	}
	if f.next().Header.Get(AssertionHeader) == "" {
		t.Error("the streamed call carries no assertion")
	}
}

// Verifies: SEC-031, LIM-001.
// A request body streams to the upstream, and one over gateway.requestBytes
// is cut; a response over gateway.responseBytes is cut too.
func TestProxyStreamsAndBoundsBodies(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.limits = mustTighten(t, mustTighten(t, f.limits, limits.GatewayRequestBytes, 64), limits.GatewayResponseBytes, 64)
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		if r.URL.Path == "/api/big" {
			_, _ = w.Write(make([]byte, 100))
			return
		}
		_, _ = io.WriteString(w, strings.Repeat("x", int(n)))
	}
	srv := f.front()

	resp, err := http.Post(srv.URL+"/gw/upload", "application/octet-stream", strings.NewReader(strings.Repeat("a", 40)))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || len(body) != 40 {
		t.Errorf("a body within the limit: %d, %d bytes echoed", resp.StatusCode, len(body))
	}
	f.next()

	resp, err = http.Post(srv.URL+"/gw/upload", "application/octet-stream", strings.NewReader(strings.Repeat("a", 1000)))
	if err == nil {
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 && len(body) >= 1000 {
			t.Errorf("a body over the limit was forwarded whole (status %d, %d bytes)", resp.StatusCode, len(body))
		}
	}

	resp, err = http.Get(srv.URL + "/gw/big")
	if err == nil {
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr == nil && len(body) > 64 {
			t.Errorf("a response over the limit arrived whole: %d bytes", len(body))
		}
	}
}

// Verifies: SEC-030, SEC-031.
// A WebSocket upgrade passes through the gateway: the device's frames
// reach the upstream and the upstream's reach the device.
func TestProxyCarriesWebSocketUpgrade(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" {
			http.Error(w, "no upgrade", http.StatusBadRequest)
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Accept: accepted\r\n\r\n")
		_ = rw.Flush()
		line, _ := rw.ReadString('\n')
		_, _ = rw.WriteString("echo:" + line)
		_ = rw.Flush()
	}
	srv := f.front()
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(conn, "GET /gw/socket HTTP/1.1\r\nHost: x\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: abc\r\nAuthorization: DPoP secret\r\n\r\n")
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil || !strings.Contains(status, "101") {
		t.Fatalf("status line = %q, %v", status, err)
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	_, _ = io.WriteString(conn, "hello\n")
	echo, err := br.ReadString('\n')
	if err != nil || echo != "echo:hello\n" {
		t.Errorf("echo = %q, %v", echo, err)
	}
	got := f.next()
	if got.Header.Get("Authorization") != "" || got.Header.Get(AssertionHeader) == "" {
		t.Errorf("headers at the upstream: %v", got.Header)
	}
}

// Verifies: SEC-031.
// A refused streaming call never reaches the upstream.
func TestProxyRefusesBeforeConnecting(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	srv := f.front()
	resp, err := http.Get(srv.URL + "/gw/%2e%2e/admin")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d", resp.StatusCode)
	}
	f.none()
}
