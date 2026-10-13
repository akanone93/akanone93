// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/egress"
	"github.com/nightCode42/plux3/backend/internal/fnhost"
	"github.com/nightCode42/plux3/backend/internal/fnpb"
)

// Verifies: FN-012, FN-018.
// Key-value calls reach the api role with the invocation's grant, and only
// when the capability is granted.
func TestKeyValueCarriesTheGrant_FN_012(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	msg := h.request("k\nv")

	// Not granted: the probe sees -2 and the api role is never called.
	resp, err := h.call(1, msg)
	if err != nil {
		t.Fatal(err)
	}
	out := outputJSON(t, resp)
	if out["put"] != float64(-2) || out["get"] != float64(-2) {
		t.Errorf("without the capability: %v", out)
	}
	if n := len(h.api.calls("KvPut")) + len(h.api.calls("KvGet")) + len(h.api.calls("KvDelete")); n != 0 {
		t.Errorf("%d key-value calls reached the api role without a grant of the capability", n)
	}

	msg.Capabilities.Kv = true
	resp, err = h.call(1, msg)
	if err != nil {
		t.Fatal(err)
	}
	out = outputJSON(t, resp)
	if out["put"] != float64(0) || out["get"] != "v" || out["delete"] != float64(0) || out["after"] != float64(-1) {
		t.Errorf("round trip = %v", out)
	}
	for _, op := range []string{"KvPut", "KvGet", "KvDelete"} {
		if got := h.api.calls(op); !slices.Contains(got, "grant-1") || len(got) < 1 {
			t.Errorf("%s grants = %v", op, got)
		}
	}
	if !slices.Contains(h.api.traceFrom, msg.Traceparent) {
		t.Errorf("trace context was not forwarded: %v", h.api.traceFrom)
	}
}

// Verifies: FN-012.
// Failures of the api role map to the statuses of the ABI: not found is -1,
// a limit is -3, unavailable is -7, anything else is an upstream failure (-5).
func TestKeyValueFailuresMapToStatuses_FN_012(t *testing.T) {
	t.Parallel()
	for code, want := range map[connect.Code]float64{
		connect.CodeNotFound: -1, connect.CodeResourceExhausted: -3, connect.CodeUnavailable: -7,
		connect.CodeInternal: -5, connect.CodePermissionDenied: -5,
	} {
		t.Run(code.String(), func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, harnessOptions{})
			h.api.mu.Lock()
			h.api.failWith = code
			h.api.mu.Unlock()
			msg := h.request("k\nv")
			msg.Capabilities.Kv = true
			resp, err := h.call(1, msg)
			if err != nil {
				t.Fatal(err)
			}
			if got := outputJSON(t, resp)["put"]; got != want {
				t.Errorf("put = %v, want %v", got, want)
			}
		})
	}
}

// Verifies: FN-012, ADR-0061.
// A secret is returned only for a name granted to the function, travels with
// the grant, and is never logged.
func TestSecretsAreGrantedByName_FN_012(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.api.secrets["db"] = []byte("s3cret-value")
	msg := h.request("db")
	msg.Capabilities.Secrets = []string{"db"}

	resp, err := h.call(3, msg)
	if err != nil {
		t.Fatal(err)
	}
	if got := outputJSON(t, resp)["value"]; got != "s3cret-value" {
		t.Errorf("secret = %v", got)
	}
	if got := h.api.calls("GetSecret"); len(got) != 1 || got[0] != "grant-1" {
		t.Errorf("GetSecret grants = %v", got)
	}

	for name, input := range map[string]string{"not granted": "other", "missing": "absent"} {
		before := len(h.api.calls("GetSecret"))
		msg := h.request(input)
		msg.Capabilities.Secrets = []string{"absent"}
		resp, err := h.call(3, msg)
		if err != nil {
			t.Fatal(err)
		}
		want, calls := float64(-2), 0
		if name == "missing" {
			want, calls = -1, 1
		}
		if got := outputJSON(t, resp)["status"]; got != want {
			t.Errorf("%s: status = %v, want %v", name, got, want)
		}
		if got := len(h.api.calls("GetSecret")) - before; got != calls {
			t.Errorf("%s: %d calls reached the api role, want %d", name, got, calls)
		}
	}
	if strings.Contains(h.logs.String(), "s3cret-value") {
		t.Error("the secret was logged")
	}
}

// Verifies: FN-012, SEC-026.
// Claims and the context reach the function only as granted.
func TestClaimsAndContext_FN_012(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	msg := h.request("{}")
	msg.Claims = []byte(`{"sub":"user-1"}`)

	resp, err := h.call(4, msg)
	if err != nil {
		t.Fatal(err)
	}
	if got := outputJSON(t, resp)["status"]; got != float64(-2) {
		t.Errorf("claims without the capability: %v", got)
	}
	msg.Capabilities.Claims = true
	resp, err = h.call(4, msg)
	if err != nil {
		t.Fatal(err)
	}
	if got := outputJSON(t, resp)["sub"]; got != "user-1" {
		t.Errorf("claims = %v", got)
	}

	resp, err = h.call(6, h.request("REGION"))
	if err != nil {
		t.Fatal(err)
	}
	ctxInfo, _ := outputJSON(t, resp)["context"].(map[string]any)
	if ctxInfo["app"] != "app-1" || ctxInfo["environment"] != "prod" || ctxInfo["assuranceLevel"] != "AL2" || ctxInfo["placement"] != "server" {
		t.Errorf("context = %v", ctxInfo)
	}
}

// Verifies: FN-011, FN-012.
// A nested call carries the grant and the depth it runs at (the caller's plus
// one); a business error comes back as the function's own result; the depth
// limit stops a call before it leaves the runner.
func TestNestedInvocationCarriesGrantAndDepth_FN_012(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.api.nested = func(req *fnpb.FunctionHostServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
		if req.GetFunction() == "ghost" {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("no such function"))
		}
		if req.GetFunction() == "bad" {
			return &fnpb.FunctionRunnerServiceInvokeResponse{Result: &fnpb.FunctionRunnerServiceInvokeResponse_BusinessError{
				BusinessError: []byte(`{"code":"c","messageKey":"k","params":{"a":1}}`),
			}}, nil
		}
		return &fnpb.FunctionRunnerServiceInvokeResponse{Result: &fnpb.FunctionRunnerServiceInvokeResponse_Output{
			Output: []byte(fmt.Sprintf(`{"echo":%s}`, req.GetInput())),
		}}, nil
	}
	msg := h.request("fmt\n{\"x\":1}")
	msg.Depth = 1
	msg.Capabilities.Functions = []string{"fmt", "bad", "ghost"}

	resp, err := h.call(5, msg)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := outputJSON(t, resp)["body"].(map[string]any)
	if outputJSON(t, resp)["tag"] != float64(0) || body["echo"] == nil {
		t.Errorf("nested output = %v", outputJSON(t, resp))
	}
	if got := h.api.calls("Invoke"); len(got) != 1 || got[0] != "grant-1" {
		t.Errorf("Invoke grants = %v", got)
	}
	if h.api.depths[0] != 2 {
		t.Errorf("nested depth = %d, want 2", h.api.depths[0])
	}

	msg.Input = []byte("bad\n{}")
	resp, err = h.call(5, msg)
	if err != nil {
		t.Fatal(err)
	}
	if out := outputJSON(t, resp); out["tag"] != float64(1) {
		t.Errorf("nested business error = %v", out)
	}

	msg.Input = []byte("ghost\n{}") // the fake answers NotFound
	resp, _ = h.call(5, msg)
	if got := outputJSON(t, resp)["status"]; got != float64(-1) {
		t.Errorf("nested unknown function = %v", got)
	}

	before := len(h.api.calls("Invoke"))
	msg.Depth = 3 // at the limit of 3
	msg.Input = []byte("fmt\n{}")
	resp, _ = h.call(5, msg)
	if got := outputJSON(t, resp)["status"]; got != float64(-3) {
		t.Errorf("at the depth limit = %v", got)
	}
	if len(h.api.calls("Invoke")) != before {
		t.Error("a call beyond the depth limit reached the api role")
	}
}

// Verifies: FN-012, FN-017.
// HTTP goes through a real egress role: the response comes back, a redirect
// is returned as it is, and the function's allowlist is what the proxy
// applies.
func TestHTTPThroughEgress_FN_012(t *testing.T) {
	t.Parallel()
	id := identity(t)
	port := upstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			w.Header().Set("Location", "http://other.test/elsewhere")
			w.WriteHeader(http.StatusFound)
		case "/big":
			_, _ = io.WriteString(w, strings.Repeat("x", 5000))
		default:
			w.Header().Set("X-Seen-Method", r.Method)
			_, _ = io.WriteString(w, "hello")
		}
	}))
	h := newHarness(t, harnessOptions{egress: startEgress(t, id, true, port), tweak: func(o *Options) { o.HTTPResponseBytes = 1000 }})
	url := func(path string) string { return fmt.Sprintf("http://upstream.test:%d%s", port, path) }
	call := func(path string, domains ...string) map[string]any {
		msg := h.request(url(path))
		msg.Capabilities.HttpDomains = domains
		resp, err := h.call(2, msg)
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		return outputJSON(t, resp)
	}

	out := call("/ok", "upstream.test")
	if out["status"] != float64(200) || out["body"] != "aGVsbG8=" {
		t.Errorf("response = %v", out)
	}
	if hdr, _ := out["headers"].(map[string]any); hdr["X-Seen-Method"] != "GET" {
		t.Errorf("headers = %v", out["headers"])
	}

	out = call("/redirect", "upstream.test")
	if hdr, _ := out["headers"].(map[string]any); out["status"] != float64(302) || hdr["Location"] != "http://other.test/elsewhere" {
		t.Errorf("redirect was followed or hidden: %v", out)
	}
	if out = call("/big", "upstream.test"); out["status"] != float64(-3) {
		t.Errorf("a body above the bound: %v", out)
	}
	if out = call("/ok", "api.example.com"); out["status"] != float64(-4) {
		t.Errorf("a domain off the allowlist: %v", out)
	}
	if out = call("/ok"); out["status"] != float64(-2) {
		t.Errorf("HTTP without the capability: %v", out)
	}
}

// Verifies: FN-012, FN-017.
// What the egress role refuses or cannot reach reaches the function as -4 and
// -5.
func TestHTTPEgressFailuresMapToStatuses_FN_012(t *testing.T) {
	t.Parallel()
	id := identity(t)
	port := upstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack() //nolint:forcetypeassert // the test server supports it
		if err == nil {
			_ = conn.Close()
		}
	}))

	run := func(dialer *egress.Dialer, host string, port int) map[string]any {
		h := newHarness(t, harnessOptions{egress: dialer})
		msg := h.request(fmt.Sprintf("http://%s:%d/", host, port))
		msg.Capabilities.HttpDomains = []string{host}
		resp, err := h.call(2, msg)
		if err != nil {
			t.Fatal(err)
		}
		return outputJSON(t, resp)
	}
	if out := run(startEgress(t, id, false, port), "upstream.test", port); out["status"] != float64(-4) {
		t.Errorf("a loopback address behind an allowed name: %v", out)
	}
	if out := run(startEgress(t, id, true, port), "upstream.test", port+1); out["status"] != float64(-4) {
		t.Errorf("a port the proxy does not allow: %v", out)
	}
	if out := run(startEgress(t, id, true, port), "nowhere.test", port); out["status"] != float64(-5) {
		t.Errorf("a name that does not resolve: %v", out)
	}
	if out := run(startEgress(t, id, true, port), "upstream.test", port); out["status"] != float64(-5) {
		t.Errorf("an upstream that drops the connection: %v", out)
	}
	if out := run(&egress.Dialer{Addr: "127.0.0.1:1", Timeout: time.Second}, "upstream.test", port); out["status"] != float64(-5) {
		t.Errorf("an unreachable proxy: %v", out)
	}
}

// Verifies: FN-011.
// Outbound calls count against the limit and an HTTP request lives no longer
// than the invocation.
func TestHTTPIsBoundedByTheInvocation_FN_011(t *testing.T) {
	t.Parallel()
	id := identity(t)
	stuck := make(chan struct{})
	port := upstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stuck" {
			select {
			case <-stuck:
			case <-r.Context().Done():
			}
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(func() { close(stuck) })
	h := newHarness(t, harnessOptions{egress: startEgress(t, id, true, port)})

	msg := h.request(fmt.Sprintf("2\nhttp://upstream.test:%d/ok", port))
	msg.Capabilities.HttpDomains = []string{"upstream.test"}
	msg.Limits.HttpCalls = 1
	resp, err := h.call(22, msg)
	if err != nil {
		t.Fatal(err)
	}
	if results, _ := outputJSON(t, resp)["results"].([]any); len(results) != 2 || results[0] != float64(0) || results[1] != float64(-3) {
		t.Errorf("results = %v", results)
	}
	if resp.GetHttpCalls() != 1 {
		t.Errorf("http_calls = %d, want 1", resp.GetHttpCalls())
	}

	msg = h.request(fmt.Sprintf("http://upstream.test:%d/stuck", port))
	msg.Capabilities.HttpDomains = []string{"upstream.test"}
	msg.Limits.TimeoutMs = 300
	start := time.Now()
	_, err = h.call(2, msg)
	if err == nil || plxCode(t, err) != "PLX-7001" {
		t.Errorf("a request that outlives the invocation: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("the invocation took %v", time.Since(start))
	}
}

// Verifies: FN-012.
// The mapping of host errors to the sentinels of the ABI.
func TestErrorMapping_FN_012(t *testing.T) {
	t.Parallel()
	for code, want := range map[connect.Code]error{
		connect.CodeNotFound: fnhost.ErrNotFound, connect.CodeResourceExhausted: fnhost.ErrLimit,
		connect.CodeUnavailable: fnhost.ErrUnavailable, connect.CodeInternal: fnhost.ErrUpstream,
		connect.CodeUnauthenticated: fnhost.ErrUpstream,
	} {
		if got := mapRemote(connect.NewError(code, errors.New("x"))); !errors.Is(got, want) {
			t.Errorf("mapRemote(%v) = %v, want %v", code, got, want)
		}
	}
	for err, want := range map[error]error{
		fmt.Errorf("dial: %w", egress.ErrRefused):  fnhost.ErrEgressRefused,
		fmt.Errorf("dial: %w", egress.ErrLimited):  fnhost.ErrUnavailable,
		fmt.Errorf("dial: %w", egress.ErrUpstream): fnhost.ErrUpstream,
		errors.New("connection reset"):             fnhost.ErrUpstream,
		context.DeadlineExceeded:                   fnhost.ErrUpstream,
	} {
		if got := mapEgress(err); !errors.Is(got, want) {
			t.Errorf("mapEgress(%v) = %v, want %v", err, got, want)
		}
	}
}
