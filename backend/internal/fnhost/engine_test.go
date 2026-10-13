// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/fnhost"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// invoke runs one call with the default limits and returns the output text.
func invoke(t testing.TB, e *fnhost.Engine, m *fnhost.Module, call fnhost.Call) (string, fnhost.Stats, error) {
	t.Helper()
	if call.Limits == (fnhost.Limits{}) {
		call.Limits = defaultLimits()
	}
	out, stats, err := e.Invoke(context.Background(), m, call)
	return string(out.Output), stats, err
}

func wantCode(t testing.TB, err error, code plxerr.Code) {
	t.Helper()
	var pe *plxerr.Error
	if !errors.As(err, &pe) || pe.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}

// Verifies: FN-010.
func TestFreshInstancePerCall_FN_010(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	for range 3 {
		out, _, err := invoke(t, e, m, fnhost.Call{Index: 18})
		if err != nil || out != `{"counter":1}` {
			t.Fatalf("counter = %s, %v; a static leaked between invocations", out, err)
		}
	}
}

// Verifies: FN-010.
func TestConcurrentInvocationsDoNotShareState_FN_010(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Go(func() {
			for j := range 3 {
				input := fmt.Sprintf(`{"caller":%d,"round":%d}`, i, j)
				out, _, err := invoke(t, e, m, fnhost.Call{Index: 0, Input: []byte(input)})
				if err != nil || out != input {
					t.Errorf("echo = %q, %v; want %q", out, err, input)
				}
				out, _, err = invoke(t, e, m, fnhost.Call{Index: 18})
				if err != nil || out != `{"counter":1}` {
					t.Errorf("counter = %q, %v", out, err)
				}
			}
		})
	}
	wg.Wait()
}

// Verifies: FN-011.
func TestTimeoutIsAWallTimeLimit_FN_011(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	limits := defaultLimits()
	limits.Timeout = 150 * time.Millisecond
	began := time.Now()
	_, _, err := invoke(t, e, m, fnhost.Call{Index: 8, Limits: limits})
	wantCode(t, err, plxerr.FunctionTimeout)
	if took := time.Since(began); took > 5*time.Second {
		t.Errorf("the loop ran for %v after a limit of %v", took, limits.Timeout)
	}
}

// Verifies: FN-011.
func TestCallerDeadlineEndsTheInvocation_FN_011(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	call := fnhost.Call{Index: 8, Limits: defaultLimits()}
	_, _, err := e.Invoke(ctx, m, call)
	wantCode(t, err, plxerr.FunctionTimeout)
}

// Verifies: FN-011.
func TestCancellationIsReportedAsSuch_FN_011(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(150*time.Millisecond, cancel)
	defer timer.Stop()
	_, _, err := e.Invoke(ctx, m, fnhost.Call{Index: 8, Limits: defaultLimits()})
	wantCode(t, err, plxerr.FunctionCancelled)
}

// Verifies: FN-011.
func TestMemoryLimitCountsGrowth_FN_011(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	limits := defaultLimits()
	limits.MemoryBytes = 6 << 20
	_, stats, err := invoke(t, e, m, fnhost.Call{Index: 9, Limits: limits})
	wantCode(t, err, plxerr.FunctionMemoryExceeded)
	if stats.PeakMemoryBytes == 0 || stats.PeakMemoryBytes > limits.MemoryBytes {
		t.Errorf("PeakMemoryBytes = %d, want within (0, %d]", stats.PeakMemoryBytes, limits.MemoryBytes)
	}
}

// Verifies: FN-011.
func TestInputIsRefusedBeforeAnInstanceExists_FN_011(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	limits := defaultLimits()
	limits.InputBytes = 4
	_, stats, err := invoke(t, e, m, fnhost.Call{Index: 0, Input: []byte("12345"), Limits: limits})
	wantCode(t, err, plxerr.FunctionInputTooLarge)
	if stats.PeakMemoryBytes != 0 {
		t.Errorf("an instance was created: peak memory %d", stats.PeakMemoryBytes)
	}
}

// Verifies: FN-011.
func TestOutputOverTheLimitIsNotCopied_FN_011(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	limits := defaultLimits()
	limits.OutputBytes = 10
	out, _, err := e.Invoke(context.Background(), m, fnhost.Call{Index: 10, Input: []byte("11"), Limits: limits})
	wantCode(t, err, plxerr.FunctionOutputTooLarge)
	if out.Output != nil || out.Business != nil {
		t.Errorf("outcome = %+v, want none", out)
	}
}

// Verifies: FN-011.
func TestHTTPCallsAreCountedAndLimited_FN_011(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	host := newFakeHost()
	host.http["https://api.example.com/"] = fnhost.HTTPResponse{Status: 204}
	limits := defaultLimits()
	limits.HTTPCalls = 2
	out, stats, err := invoke(t, e, m, fnhost.Call{
		Index: 22, Input: []byte("4\nhttps://api.example.com/"), Limits: limits,
		Grants: fnhost.Grants{HTTPDomains: []string{"api.example.com"}}, Host: host,
	})
	if err != nil || out != `{"results":[0,0,-3,-3]}` {
		t.Fatalf("results = %s, %v", out, err)
	}
	if stats.HTTPCalls != 2 || host.calls != 2 {
		t.Errorf("counted %d calls, host saw %d; want 2 and 2", stats.HTTPCalls, host.calls)
	}
}

// Verifies: FN-011.
func TestLogBytesAreCountedInStats_FN_011(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	limits := defaultLimits()
	limits.LogBytes = 130
	var logs []fnhost.LogRecord
	_, stats, err := invoke(t, e, m, fnhost.Call{
		Index: 7, Input: []byte("5"), Limits: limits,
		Log: func(r fnhost.LogRecord) { logs = append(logs, r) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.LogBytes != 128 || len(logs) != 3 || logs[2].Message != fnhost.LogTruncated {
		t.Errorf("LogBytes = %d, logs = %d records; want 128 and two lines plus the marker", stats.LogBytes, len(logs))
	}
}

// Verifies: FN-012.
func TestNothingIsGrantedByDefault_FN_012(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	host := newFakeHost()
	host.kv["k"] = []byte("v")
	host.secrets["s"] = "x"
	host.http["https://api.example.com/"] = fnhost.HTTPResponse{Status: 200}
	host.invoke["f"] = fnhost.Outcome{Output: json.RawMessage(`1`)}
	tests := map[string]fnhost.Call{
		"kv":     {Index: 1, Input: []byte("k\nv")},
		"http":   {Index: 2, Input: []byte("https://api.example.com/")},
		"secret": {Index: 3, Input: []byte("s")},
		"claims": {Index: 4, Claims: json.RawMessage(`{"sub":"u"}`)},
		"invoke": {Index: 5, Input: []byte("f\n{}")},
	}
	for name, call := range tests {
		call.Host = host
		out, _, err := invoke(t, e, m, call)
		if err != nil || !strings.Contains(out, "-2") || strings.Contains(out, `"get":"v"`) {
			t.Errorf("%s: output %s, %v; want every call denied with -2", name, out, err)
		}
	}
	if host.calls != 0 {
		t.Errorf("the host was called %d times", host.calls)
	}
}

// Verifies: FN-012.
func TestNilHostServesNothing_FN_012(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	out, _, err := invoke(t, e, m, fnhost.Call{Index: 1, Input: []byte("k\nv"), Grants: fnhost.Grants{KV: true}})
	if err != nil || out != `{"put":-7,"get":-7,"delete":-7,"after":-7}` {
		t.Fatalf("output = %s, %v; want every call unavailable", out, err)
	}
	all := fnhost.Grants{
		HTTPDomains: []string{"api.example.com"}, Secrets: []string{"s"}, Functions: []string{"f"},
	}
	for index, input := range map[uint32]string{2: "https://api.example.com/", 3: "s", 5: "f\n{}"} {
		out, _, err := invoke(t, e, m, fnhost.Call{Index: index, Input: []byte(input), Grants: all})
		if err != nil || out != `{"status":-7}` {
			t.Errorf("entry %d: output = %s, %v; want unavailable", index, out, err)
		}
	}
}

// Verifies: FN-052.
func TestDeviceModuleCannotImportServerFunctions_FN_052(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	_, err := e.Compile(context.Background(), fixture(t, "probe.wasm"), fnhost.PlacementDevice)
	wantCode(t, err, plxerr.FunctionModuleInvalid)
	m := compile(t, e, "probe-device.wasm", fnhost.PlacementDevice)
	if m.Placement() != fnhost.PlacementDevice {
		t.Errorf("Placement = %q", m.Placement())
	}
	_, _, err = invoke(t, e, m, fnhost.Call{Index: 2, Input: []byte("https://api.example.com/")})
	wantCode(t, err, plxerr.FunctionInputInvalid)
}

// Verifies: FN-015.
func TestModuleIdentityIsTheSHA256(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe-device.wasm", fnhost.PlacementServer)
	if m.SHA256() != sha256.Sum256(fixture(t, "probe-device.wasm")) {
		t.Error("SHA256 differs from the digest of the module bytes")
	}
}

// Verifies: FN-015.
func TestHostErrorsOtherThanSentinelsAreUnavailable(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	host := newFakeHost()
	host.fail["kvPut"] = errors.New("database is on fire")
	out, _, err := invoke(t, e, m, fnhost.Call{Index: 1, Input: []byte("k\nv"), Grants: fnhost.Grants{KV: true}, Host: host})
	if err != nil || !strings.HasPrefix(out, `{"put":-7`) {
		t.Fatalf("output = %s, %v", out, err)
	}
}

func TestInvokeRejectsAnUnboundedCall(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	limits := defaultLimits()
	limits.Timeout = 0
	if _, _, err := invoke(t, e, m, fnhost.Call{Limits: limits}); err == nil {
		t.Error("a call without a timeout ran")
	}
	if _, _, err := invoke(t, e, m, fnhost.Call{Depth: -1}); err == nil {
		t.Error("a negative depth ran")
	}
}

func TestStatsDuration(t *testing.T) {
	t.Parallel()
	ticks := 0
	e, err := fnhost.NewEngine(context.Background(), fnhost.Config{
		Clock:          func() time.Time { ticks++; return testClock.Add(time.Duration(ticks) * time.Second) },
		Random:         &counterReader{},
		MaxMemoryBytes: 64 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close(context.Background()) }()
	m := compile(t, e, "probe.wasm", fnhost.PlacementServer)
	_, stats, err := invoke(t, e, m, fnhost.Call{Index: 0})
	if err != nil || stats.Duration != time.Second {
		t.Errorf("Duration = %v, %v; want 1s from two clock reads", stats.Duration, err)
	}
}

func TestEngineConfiguration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := func() time.Time { return testClock }
	bad := []fnhost.Config{
		{Random: &counterReader{}, MaxMemoryBytes: 1 << 20},
		{Clock: clock, MaxMemoryBytes: 1 << 20},
		{Clock: clock, Random: &counterReader{}},
		{Clock: clock, Random: &counterReader{}, MaxMemoryBytes: 1<<31 + 1},
		{Clock: clock, Random: &counterReader{}, MaxMemoryBytes: 1 << 20, CacheDir: "/dev/null/cache"},
	}
	for i, cfg := range bad {
		if e, err := fnhost.NewEngine(ctx, cfg); err == nil {
			_ = e.Close(ctx)
			t.Errorf("config %d was accepted", i)
		}
	}
}

func TestCompilationCacheAndClose(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := fnhost.Config{
		Clock: func() time.Time { return testClock }, Random: &counterReader{},
		MaxMemoryBytes: 64 << 20, CacheDir: t.TempDir(),
	}
	for range 2 {
		e, err := fnhost.NewEngine(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		m, err := e.Compile(ctx, fixture(t, "probe-device.wasm"), fnhost.PlacementServer)
		if err != nil {
			t.Fatal(err)
		}
		if out, _, err := invoke(t, e, m, fnhost.Call{Index: 0, Input: []byte("1")}); err != nil || out != "1" {
			t.Fatalf("echo = %q, %v", out, err)
		}
		if err := e.Close(ctx); err != nil {
			t.Fatal(err)
		}
		if err := e.Close(ctx); err != nil {
			t.Fatalf("second Close: %v", err)
		}
	}
}

func TestCompileRejectsAnUnknownPlacement(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	if _, err := e.Compile(context.Background(), fixture(t, "probe.wasm"), "cloud"); err == nil {
		t.Error("an unknown placement was accepted")
	}
}

func TestCompileHonoursTheCallersContext(t *testing.T) {
	t.Parallel()
	e := newEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Compile(ctx, fixture(t, "probe.wasm"), fnhost.PlacementServer); err == nil {
		t.Error("compiling with a cancelled context succeeded")
	}
}

// FuzzCompile feeds arbitrary bytes to Compile, which must refuse them
// without panicking.
func FuzzCompile(f *testing.F) {
	e := newEngine(f)
	f.Add([]byte{})
	f.Add([]byte("\x00asm\x01\x00\x00\x00"))
	f.Add(fixture(f, "probe-device.wasm"))
	f.Add(fixture(f, "probe-device.wasm")[:200])
	f.Fuzz(func(t *testing.T, data []byte) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		for _, p := range []fnhost.Placement{fnhost.PlacementServer, fnhost.PlacementDevice} {
			m, err := e.Compile(ctx, data, p)
			if err == nil {
				_ = m.Close(ctx)
			}
		}
	})
}

// FuzzInvokeEcho round-trips arbitrary input through the guest.
func FuzzInvokeEcho(f *testing.F) {
	e := newEngine(f)
	m := compile(f, e, "probe-device.wasm", fnhost.PlacementServer)
	f.Add([]byte{})
	f.Add([]byte(`{"a":[1,2,3]}`))
	f.Add([]byte{0, 0xff, '\n', 0x80})
	f.Fuzz(func(t *testing.T, input []byte) {
		limits := defaultLimits()
		limits.InputBytes = 1 << 16
		out, _, err := e.Invoke(context.Background(), m, fnhost.Call{Index: 0, Input: input, Limits: limits})
		if len(input) > limits.InputBytes {
			wantCode(t, err, plxerr.FunctionInputTooLarge)
			return
		}
		if err != nil || string(out.Output) != string(input) {
			t.Fatalf("echo(%q) = %q, %v", input, out.Output, err)
		}
	})
}
