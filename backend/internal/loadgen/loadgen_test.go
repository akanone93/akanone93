// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package loadgen_test

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/loadgen"
	"github.com/nightCode42/plux3/backend/internal/loadgen/loadgentest"
)

// fastPacing is a pacing that gives every task several rounds in a second.
func fastPacing() loadgen.Pacing {
	return loadgen.Pacing{
		Manifest: 150 * time.Millisecond, Control: 100 * time.Millisecond,
		Telemetry: 120 * time.Millisecond, Function: 200 * time.Millisecond, Jitter: 0.2,
	}
}

func config(srv *loadgentest.Server, devices int, d time.Duration) loadgen.Config {
	return loadgen.Config{
		BaseURL: srv.URL, AppID: "app", Environment: "staging", Devices: devices, Duration: d,
		Pacing: fastPacing(), Function: &loadgen.Function{Name: "demo.echo", Version: 1, Input: []byte(`{}`)}, Seed: 7,
	}
}

// Verifies: DEP-051.
// The default pacing is the runtime's: the control document every five
// minutes (SYN-060). Scaling divides every interval and keeps the jitter.
func TestDefaultPacingAndScale_DEP_051(t *testing.T) {
	t.Parallel()
	p := loadgen.DefaultPacing()
	if p.Control != 5*time.Minute || p.Telemetry != time.Minute {
		t.Fatalf("pacing = %+v", p)
	}
	s := p.Scaled(60)
	if s.Control != 5*time.Second || s.Telemetry != time.Second || s.Manifest != 10*time.Second || s.Jitter != p.Jitter {
		t.Errorf("scaled by 60 = %+v", s)
	}
	if got := p.Scaled(0); got != p {
		t.Errorf("a scale of 0 must leave the pacing as it is: %+v", got)
	}
}

// Verifies: DEP-051.
// A run that cannot make sense is refused before any request is made.
func TestConfigIsValidated_DEP_051(t *testing.T) {
	t.Parallel()
	good := loadgen.Config{BaseURL: "http://x", AppID: "a", Devices: 1, Duration: time.Second}
	for name, mutate := range map[string]func(*loadgen.Config){
		"no server":        func(c *loadgen.Config) { c.BaseURL = "" },
		"no app":           func(c *loadgen.Config) { c.AppID = "" },
		"no device":        func(c *loadgen.Config) { c.Devices = 0 },
		"no duration":      func(c *loadgen.Config) { c.Duration = 0 },
		"zero interval":    func(c *loadgen.Config) { c.Pacing = loadgen.Pacing{Manifest: time.Second, Telemetry: time.Second} },
		"jitter too large": func(c *loadgen.Config) { c.Pacing = loadgen.DefaultPacing(); c.Pacing.Jitter = 2 },
	} {
		c := good
		mutate(&c)
		if _, err := loadgen.NewFleet(c); err == nil {
			t.Errorf("%s: NewFleet accepted the configuration", name)
		}
	}
	if _, err := loadgen.NewFleet(good); err != nil {
		t.Errorf("a valid configuration: %v", err)
	}
}

// Verifies: DEP-051, NFR-023.
// A few devices for a short time run every kind of request against an
// endpoint that verifies their DPoP proofs, without an error, and the
// report counts each of them and has the documented shape.
func TestRunAgainstFakeEndpoint_DEP_051(t *testing.T) {
	t.Parallel()
	srv := loadgentest.New(t)
	cfg := config(srv, 4, 1500*time.Millisecond)
	cfg.PushTokens = true
	cfg.ForwardedFor = true

	report, err := loadgen.Run(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if report.Schema != loadgen.ReportSchema || report.Devices != 4 || report.Registered != 4 || report.Errors() != 0 {
		t.Fatalf("report = %+v", report)
	}
	for _, name := range []string{loadgen.OpRegister, loadgen.OpManifest, loadgen.OpControl, loadgen.OpTelemetry, loadgen.OpFunction} {
		op, ok := report.Operations[name]
		if !ok || op.Requests == 0 {
			t.Errorf("%s was never requested: %+v", name, report.Operations)
			continue
		}
		if op.LatencyMS.P50 <= 0 || op.LatencyMS.P50 > op.LatencyMS.P95 || op.LatencyMS.P95 > op.LatencyMS.P99*1.05 || op.LatencyMS.Max <= 0 {
			t.Errorf("%s latency = %+v", name, op.LatencyMS)
		}
	}
	if report.Operations[loadgen.OpRegister].Requests != 4 {
		t.Errorf("registrations = %d, want 4", report.Operations[loadgen.OpRegister].Requests)
	}
	if got := report.Operations[loadgen.OpControl].RatePerSecond; got <= 0 {
		t.Errorf("the control rate = %v", got)
	}
	if report.ControlApplied != 4 {
		t.Errorf("control_applied = %d, want one per device", report.ControlApplied)
	}
	events := srv.Events()
	for _, want := range []string{"session_start", "control_applied", "screen_view", "action_run"} {
		if !slices.Contains(events, want) {
			t.Errorf("no %s event was sent: %v", want, events)
		}
	}
	if srv.Calls("DeviceService/RegisterPushToken") != 4 {
		t.Errorf("push tokens registered = %d", srv.Calls("DeviceService/RegisterPushToken"))
	}
}

// Verifies: DEP-051, REL-053.
// A run that injects errors reports an error event, the synthetic regression
// a rollout's health gate trips on; an ordinary run sends none, and a run
// limited to another release sends none either.
func TestInjectErrorsSendsErrorEvents_REL_053(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		inject  bool
		release int64
		want    bool
	}{
		"off":               {inject: false, want: false},
		"every device":      {inject: true, want: true},
		"the named release": {inject: true, release: 7, want: true},
		"another release":   {inject: true, release: 8, want: false},
	} {
		srv := loadgentest.New(t)
		srv.SetReleaseSequence(7)
		cfg := config(srv, 2, time.Second)
		cfg.InjectErrors, cfg.ErrorRelease = tc.inject, tc.release
		if _, err := loadgen.Run(t.Context(), cfg); err != nil {
			t.Fatal(err)
		}
		if got := slices.Contains(srv.Events(), "error"); got != tc.want {
			t.Errorf("%s: error event sent = %v, want %v", name, got, tc.want)
		}
	}
}

// Verifies: DEP-051.
// The report is JSON with the documented keys, and nothing but counts,
// rates and latencies.
func TestReportFormat_DEP_051(t *testing.T) {
	t.Parallel()
	srv := loadgentest.New(t)
	report, err := loadgen.Run(t.Context(), config(srv, 2, 600*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema", "devices", "registered", "durationSeconds", "pacingScale", "operations", "controlApplied"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("the report lacks %q: %s", key, raw)
		}
	}
	ops, _ := doc["operations"].(map[string]any)
	manifest, _ := ops[loadgen.OpManifest].(map[string]any)
	for _, key := range []string{"requests", "errors", "ratePerSecond", "realRatePerSecond", "latencyMs"} {
		if _, ok := manifest[key]; !ok {
			t.Errorf("the manifest operation lacks %q: %s", key, raw)
		}
	}
	if len(doc) != 7 {
		t.Errorf("the report has %d keys, want exactly the documented seven: %s", len(doc), raw)
	}
}

// Verifies: DEP-051.
// Requests the server refuses are counted by code and make the run's errors
// non-zero; they do not stop the run.
func TestErrorsAreCountedByCode_DEP_051(t *testing.T) {
	t.Parallel()
	srv := loadgentest.New(t)
	srv.FailTelemetry(connect.CodeUnavailable)
	report, err := loadgen.Run(t.Context(), config(srv, 2, 600*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	tel := report.Operations[loadgen.OpTelemetry]
	if tel.Errors == 0 || tel.Errors != tel.Requests || tel.ErrorCodes["unavailable"] != tel.Errors {
		t.Errorf("telemetry = %+v", tel)
	}
	if report.Operations[loadgen.OpManifest].Errors != 0 {
		t.Errorf("manifest errors = %d", report.Operations[loadgen.OpManifest].Errors)
	}
}

// Verifies: DEP-051.
// Pacing alone sets the load: with no function configured none is called,
// and the request counts follow the intervals.
func TestPacingSetsTheLoad_DEP_051(t *testing.T) {
	t.Parallel()
	srv := loadgentest.New(t)
	cfg := config(srv, 3, 1200*time.Millisecond)
	cfg.Function = nil
	report, err := loadgen.Run(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := report.Operations[loadgen.OpFunction]; ok {
		t.Error("a function was called although none is configured")
	}
	// Per device: startup plus about 1.2 s / interval, with timer slack.
	control := report.Operations[loadgen.OpControl].Requests / 3
	manifest := report.Operations[loadgen.OpManifest].Requests / 3
	if control < 5 || control > 16 || manifest < 3 || manifest > 12 || control <= manifest {
		t.Errorf("per device: control %d, manifest %d", control, manifest)
	}
}

// Verifies: NFR-024, SYN-060.
// A device that fetches a document with a newer version reports
// control_applied once for it; a nudge, which stands for a silent push,
// makes it check at once instead of at its next interval.
func TestControlAppliedAndNudge_NFR_024(t *testing.T) {
	t.Parallel()
	srv := loadgentest.New(t)
	var (
		mu      sync.Mutex
		applied []loadgen.Applied
	)
	cfg := config(srv, 3, 3*time.Second)
	cfg.Pacing = loadgen.Pacing{Manifest: time.Hour, Control: time.Hour, Telemetry: time.Hour, Jitter: 0}
	cfg.PushTokens = true
	cfg.OnApplied = func(a loadgen.Applied) {
		mu.Lock()
		applied = append(applied, a)
		mu.Unlock()
	}
	fleet, err := loadgen.NewFleet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := fleet.Register(t.Context()); err != nil {
		t.Fatal(err)
	}
	done := make(chan loadgen.Report, 1)
	go func() { done <- fleet.Run(t.Context()) }()

	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(applied) == 3 })
	srv.SetControlVersion(2)
	if fleet.Nudge("no-such-token") {
		t.Error("a token of no device was accepted")
	}
	for _, token := range srv.PushTokens() {
		if !fleet.Nudge(token) {
			t.Errorf("token %q is unknown to the fleet", token)
		}
	}
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(applied) == 6 })

	report := <-done
	mu.Lock()
	defer mu.Unlock()
	versions := map[int64]int{}
	for _, a := range applied {
		versions[a.Version]++
		if a.DeviceID == "" || a.At.IsZero() {
			t.Errorf("applied = %+v", a)
		}
	}
	if versions[1] != 3 || versions[2] != 3 || report.ControlApplied != 6 || report.Errors() != 0 {
		t.Errorf("versions %v, report %+v", versions, report)
	}
	if got := srv.Calls("ManifestService/GetSignedControl"); got != 6 {
		t.Errorf("control fetches = %d, want 3 at start and 3 nudged", got)
	}
}

// Verifies: DEP-051.
// When no device can register, Run says so rather than reporting an empty
// success.
func TestNoRegistrationIsAnError_DEP_051(t *testing.T) {
	t.Parallel()
	cfg := loadgen.Config{BaseURL: "http://127.0.0.1:1", AppID: "a", Devices: 2, Duration: time.Second}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	report, err := loadgen.Run(ctx, cfg)
	if err == nil {
		t.Fatal("Run succeeded without a server")
	}
	if got := report.Operations[loadgen.OpRegister]; got.Errors != 2 || got.ErrorCodes["unavailable"] != 2 {
		t.Errorf("register = %+v", got)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("the condition was not met in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
