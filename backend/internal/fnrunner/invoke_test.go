// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
)

// outputJSON decodes the output of a successful invocation into a map.
func outputJSON(t *testing.T, resp *fnpb.FunctionRunnerServiceInvokeResponse) map[string]any {
	t.Helper()
	raw, ok := resp.GetResult().(*fnpb.FunctionRunnerServiceInvokeResponse_Output)
	if !ok {
		t.Fatalf("result = %T, want an output", resp.GetResult())
	}
	var out map[string]any
	if err := json.Unmarshal(raw.Output, &out); err != nil {
		t.Fatalf("output %q: %v", raw.Output, err)
	}
	return out
}

// Verifies: FN-010, FN-032.
// An invocation returns the function's output with its memory peak, runs in a
// fresh instance every time, and is counted.
func TestInvokeRunsInAFreshInstance_FN_010(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	resp, err := h.runner.Invoke(context.Background(), connect.NewRequest(h.request(`{"hello":"world"}`)))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := outputJSON(t, resp.Msg); got["hello"] != "world" {
		t.Errorf("echo = %v", got)
	}
	if resp.Msg.GetMemoryPeakBytes() == 0 {
		t.Error("memory_peak_bytes is zero")
	}

	for range 3 {
		out, err := h.call(18, h.request("{}"))
		if err != nil {
			t.Fatal(err)
		}
		if got := outputJSON(t, out); got["counter"] != float64(1) {
			t.Errorf("a static counter = %v in a later call: instances are shared", got["counter"])
		}
	}
	if got := h.reg.counter(t, "plux_function_invocations_total", resultOK); got != 4 {
		t.Errorf("ok counter = %v, want 4", got)
	}
	if got := h.reg.samples(t, "plux_function_duration_seconds"); got != 4 {
		t.Errorf("duration samples = %d, want 4", got)
	}
	if got := h.reg.samples(t, "plux_function_memory_peak_bytes"); got != 4 {
		t.Errorf("memory samples = %d, want 4", got)
	}
}

// Verifies: FN-010, FN-022.
// A business error is a result, not a failure of the runner.
func TestInvokeReturnsBusinessErrors_FN_010(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	resp, err := h.call(12, h.request("x"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	raw, ok := resp.GetResult().(*fnpb.FunctionRunnerServiceInvokeResponse_BusinessError)
	if !ok {
		t.Fatalf("result = %T", resp.GetResult())
	}
	var be struct {
		Code, MessageKey string
		Params           map[string]string
	}
	if err := json.Unmarshal(raw.BusinessError, &be); err != nil || be.Code != "probe.rejected" || be.MessageKey != "probe.rejected" || be.Params["input"] != "x" {
		t.Errorf("business error = %s (%v)", raw.BusinessError, err)
	}
	if got := h.reg.counter(t, "plux_function_invocations_total", resultBusinessError); got != 1 {
		t.Errorf("business_error counter = %v", got)
	}
}

// Verifies: FN-010, SEC-092.
// A system error reaches the caller with its Plux code, is counted under its
// lower-case reason, and its log line names the function but never the
// input, the output or the claims.
func TestInvokeSystemErrorsAreCodedAndLoggedWithoutPayloads_FN_010(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	msg := h.request("TOP-SECRET-INPUT")
	msg.Claims = []byte(`{"sub":"CLAIMS-MARKER"}`)
	msg.Capabilities.Claims = true

	_, err := h.call(11, msg) // the probe panics
	if err == nil {
		t.Fatal("a trapping function succeeded")
	}
	if got := plxCode(t, err); got != "PLX-7007" {
		t.Errorf("code = %s (%v)", got, err)
	}
	if got := h.reg.counter(t, "plux_function_invocations_total", "function_trapped"); got != 1 {
		t.Errorf("function_trapped counter = %v", got)
	}
	logs := h.logs.String()
	for _, want := range []string{`"app":"app-1"`, `"environment":"prod"`, `"function":"demo.probe"`, `"version":3`, `"code":"PLX-7007"`} {
		if !strings.Contains(logs, want) {
			t.Errorf("log %q does not contain %s", logs, want)
		}
	}
	for _, leak := range []string{"TOP-SECRET-INPUT", "CLAIMS-MARKER", "grant-1"} {
		if strings.Contains(logs, leak) {
			t.Errorf("log leaks %q", leak)
		}
	}
}

// Verifies: FN-010.
// A request without resolved limits is refused, not run unlimited.
func TestInvokeRefusesMissingLimits_FN_010(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	msg := h.request("{}")
	msg.Limits = nil
	if _, err := h.call(0, msg); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("no limits: %v", err)
	}
	msg = h.request("{}")
	msg.Limits.TimeoutMs = 0
	if _, err := h.call(0, msg); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("zero timeout: %v", err)
	}
}

// Verifies: FN-011.
// The limits of the request are enforced end to end: wall time, memory, input
// and output.
func TestInvokeEnforcesLimits_FN_011(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{tweak: func(o *Options) { o.MaxMemoryBytes = 16 << 20 }})
	cases := []limitCase{
		{"endless loop", 8, "", func(m *fnpb.FunctionRunnerServiceInvokeRequest) { m.Limits.TimeoutMs = 200 }, "PLX-7001", connect.CodeDeadlineExceeded},
		{"memory growth", 9, "", func(m *fnpb.FunctionRunnerServiceInvokeRequest) { m.Limits.MemoryBytes = 8 << 20 }, "PLX-7002", connect.CodeResourceExhausted},
		{"memory above the runner's maximum is clamped", 9, "", func(m *fnpb.FunctionRunnerServiceInvokeRequest) { m.Limits.MemoryBytes = 512 << 20 }, "PLX-7002", connect.CodeResourceExhausted},
		{"memory below the module's start", 0, "{}", func(m *fnpb.FunctionRunnerServiceInvokeRequest) { m.Limits.MemoryBytes = 65536 }, "PLX-7002", connect.CodeResourceExhausted},
		{"input too large", 0, "0123456789", func(m *fnpb.FunctionRunnerServiceInvokeRequest) { m.Limits.InputBytes = 4 }, "PLX-7022", connect.CodeInvalidArgument},
		{"output too large", 10, "2000", func(m *fnpb.FunctionRunnerServiceInvokeRequest) { m.Limits.OutputBytes = 1000 }, "PLX-7004", connect.CodeResourceExhausted},
	}
	t.Run("cases", func(t *testing.T) { runLimitCases(t, h, cases) })
	if got := h.reg.counter(t, "plux_function_invocations_total", "function_timeout"); got != 1 {
		t.Errorf("function_timeout counter = %v", got)
	}
}

// limitCase is one row of TestInvokeEnforcesLimits_FN_011.
type limitCase struct {
	name  string
	entry uint32
	input string
	tweak func(*fnpb.FunctionRunnerServiceInvokeRequest)
	code  string
	as    connect.Code
}

// runLimitCases runs the rows in parallel and returns when all are done.
func runLimitCases(t *testing.T, h *harness, cases []limitCase) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			msg := h.request(tc.input)
			tc.tweak(msg)
			start := time.Now()
			_, err := h.call(tc.entry, msg)
			if err == nil {
				t.Fatal("the limit was not enforced")
			}
			if got := plxCode(t, err); got != tc.code {
				t.Errorf("code = %s, want %s (%v)", got, tc.code, err)
			}
			if connect.CodeOf(err) != tc.as {
				t.Errorf("Connect code = %v, want %v", connect.CodeOf(err), tc.as)
			}
			if time.Since(start) > 5*time.Second {
				t.Errorf("took %v", time.Since(start))
			}
		})
	}
}

// Verifies: FN-010, FN-011.
// Above the concurrency bound the next invocation is refused with PLX-8040,
// not queued, and the slot is free again afterwards.
func TestInvokeBoundsConcurrency_FN_010(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{tweak: func(o *Options) { o.MaxConcurrent = 1 }})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := h.runner.invoke(ctx, h.request(""), 8) // loops until cancelled
		done <- err
	}()
	eventually(t, "the first invocation to hold the slot", func() bool { return len(h.runner.slots) == 1 })

	_, err := h.call(0, h.request("{}"))
	if err == nil || plxCode(t, err) != "PLX-8040" || connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("second invocation: %v", err)
	}
	if got := h.reg.counter(t, "plux_function_invocations_total", "rate_limited"); got != 1 {
		t.Errorf("rate_limited counter = %v", got)
	}
	cancel()
	if err := <-done; err == nil || plxCode(t, err) != "PLX-7021" {
		t.Errorf("cancelled invocation: %v", err)
	}
	if _, err := h.call(0, h.request("{}")); err != nil {
		t.Errorf("after the slot was released: %v", err)
	}
}
