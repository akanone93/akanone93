// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/loadgen"
	"github.com/nightCode42/plux3/backend/internal/push"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// fakeFCM stands in for the FCM HTTP v1 endpoint: a message for a push
// token reaches the device that holds it, as the provider would deliver it
// and the host app pass it to Plux.handleSilentPush.
type fakeFCM struct {
	srv   *httptest.Server
	fleet atomic.Pointer[loadgen.Fleet]
	sent  atomic.Int64
}

func newFakeFCM(t *testing.T) *fakeFCM {
	t.Helper()
	f := &fakeFCM{}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake", "expires_in": 3600})
	})
	mux.HandleFunc("/v1/projects/bench/messages:send", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Message struct {
				Token string `json:"token"`
			} `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.sent.Add(1)
		if fleet := f.fleet.Load(); fleet != nil {
			fleet.Nudge(body.Message.Token)
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// credentials is the service account key file the server reads from the
// environment's secrets; its key signs only the assertion the fake accepts.
func (*fakeFCM) credentials(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(push.ServiceAccount{
		ProjectID: "bench", ClientEmail: "bench@example.iam",
		PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// propagationResult is the outcome of one mode of the benchmark.
type propagationResult struct {
	Mode                string  `json:"mode"`
	Devices             int     `json:"devices"`
	PollIntervalSeconds float64 `json:"pollIntervalSeconds"`
	P50Seconds          float64 `json:"p50Seconds"`
	P95Seconds          float64 `json:"p95Seconds"`
	MaxSeconds          float64 `json:"maxSeconds"`
	Pushes              int64   `json:"pushesSent"`
}

// percentile returns the p-th percentile, 0 to 1, of sorted by nearest rank.
func percentile(sorted []time.Duration, p float64) time.Duration {
	rank := int(p*float64(len(sorted))+0.999999) - 1
	return sorted[max(0, min(rank, len(sorted)-1))]
}

// envIDOf returns the identifier of the app's environment with the key.
func envIDOf(t *testing.T, st *stack, key string) string {
	t.Helper()
	envs, err := st.svc.Tenancy.ListEnvironments(t.Context(), st.owner, st.app.ID, tenancy.Page{Size: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range envs {
		if e.Key == key {
			return e.ID
		}
	}
	t.Fatalf("no environment %q", key)
	return ""
}

// applications records when each device reports a version newer than the
// one it started with.
type applications struct {
	mu       sync.Mutex
	baseline int64
	at       map[string]time.Time
}

func (a *applications) record(ap loadgen.Applied) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if ap.Version > a.baseline {
		if _, seen := a.at[ap.DeviceID]; !seen {
			a.at[ap.DeviceID] = ap.At
		}
	}
}

func (a *applications) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.at)
}

// durations returns, sorted, the time each device took from since.
func (a *applications) durations(since time.Time) []time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]time.Duration, 0, len(a.at))
	for _, at := range a.at {
		out = append(out, at.Sub(since))
	}
	slices.Sort(out)
	return out
}

// propagationRun measures one mode: a fleet that has applied the current
// control document, a change through the CLI, and the time until every
// device reports the new version.
func propagationRun(t *testing.T, st *stack, project string, fcm *fakeFCM, withPush bool, devices int, poll time.Duration, change []string) propagationResult {
	t.Helper()
	baseline := signedControl(t, st, project).SignedVersion
	apps := &applications{baseline: baseline, at: map[string]time.Time{}}
	fleet, err := loadgen.NewFleet(loadgen.Config{
		BaseURL: st.server, AppID: st.app.ID, Environment: "staging", Devices: devices, Duration: time.Hour,
		Concurrency: 8, ForwardedFor: true, PushTokens: withPush, Seed: 42, OnApplied: apps.record,
		Pacing: loadgen.Pacing{Manifest: time.Hour, Control: poll, Telemetry: time.Hour},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fleet.Register(t.Context()); err != nil {
		t.Fatal(err)
	}
	fcm.fleet.Store(fleet)
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); fleet.Run(ctx) }()
	defer func() { stop(); <-done }()

	// Wait until every device holds the baseline: only then does a report of
	// a newer version mean the change arrived.
	settled := func(n int) bool { return fleet.Report(time.Second).ControlApplied >= int64(n) }
	waitUntil(t, 30*time.Second, func() bool { return settled(devices) }, "every device to apply the baseline")

	pushesBefore := fcm.sent.Load()
	changedAt := time.Now()
	st.run(t, 0, change...)
	waitUntil(t, 3*poll+30*time.Second, func() bool { return apps.count() == devices }, "every device to apply the change")
	d := apps.durations(changedAt)
	mode := "poll"
	if withPush {
		mode = "push"
	}
	return propagationResult{
		Mode: mode, Devices: devices, PollIntervalSeconds: poll.Seconds(),
		P50Seconds: percentile(d, 0.5).Seconds(), P95Seconds: percentile(d, 0.95).Seconds(), MaxSeconds: d[len(d)-1].Seconds(),
		Pushes: fcm.sent.Load() - pushesBefore,
	}
}

func waitUntil(t *testing.T, limit time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// histogramCount returns the number of samples of
// plux_control_propagation_seconds that the running server's metrics
// endpoint reports.
func histogramCount(t *testing.T, st *stack) uint64 {
	t.Helper()
	res, err := http.Get(st.server + "/metrics") //nolint:noctx,gosec // a test scrapes its own loopback server
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var n uint64
	lines := bufio.NewScanner(res.Body)
	for lines.Scan() {
		line := lines.Text()
		if !strings.HasPrefix(line, "plux_control_propagation_seconds_count") {
			continue
		}
		v, err := strconv.ParseFloat(line[strings.LastIndexByte(line, ' ')+1:], 64)
		if err != nil {
			t.Fatalf("metrics line %q: %v", line, err)
		}
		n += uint64(v)
	}
	if err := lines.Err(); err != nil {
		t.Fatal(err)
	}
	return n
}

// benchDevices and benchPoll read the benchmark's scale from the
// environment: the defaults keep the test to about a minute.
func benchDevices() int {
	if v, err := strconv.Atoi(os.Getenv("PLUX_BENCH_DEVICES")); err == nil && v > 0 {
		return v
	}
	return 20
}

func benchPoll() time.Duration {
	if v, err := time.ParseDuration(os.Getenv("PLUX_BENCH_POLL")); err == nil && v > 0 {
		return v
	}
	return 15 * time.Second
}

// Verifies: NFR-024, OBS-005, SYN-060.
// A switch change reaches a fleet of devices that poll for the control
// document within the poll interval and, with silent push through a fake
// FCM endpoint, within seconds; the server records every device's time in
// plux_control_propagation_seconds. The p50 and p95 of both modes are
// printed as JSON (and written to PLUX_BENCH_CONTROL_OUT when set), which is
// what `make bench-control` commits to docs/benchmarks/load.md.
func TestBenchControlPropagation_NFR_024(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the CLI and runs a fleet")
	}
	fcm := newFakeFCM(t)
	st := startStackWith(t, "127.0.0.1:0", stackOptions{
		// Every device names its own address, so the per-address limits
		// apply per device as in the field.
		serverYAML: "  trustedProxies: [127.0.0.1/32]\n",
		extraYAML:  "push:\n  enabled: true\n",
		pushEnds:   &push.Endpoints{FCMSend: fcm.srv.URL, FCMToken: fcm.srv.URL + "/token"},
		pushHTTP:   fcm.srv.Client(),
	})
	project := st.project(t, "loan-calculator")
	delegateControl(t, st, "staging")
	st.run(t, 0, "publish", "-C", project, "--promote", "staging", "--json")
	if _, err := st.svc.Tenancy.SetSecret(t.Context(), st.owner, envIDOf(t, st, "staging"), push.SecretFCM, fcm.credentials(t)); err != nil {
		t.Fatal(err)
	}
	st.run(t, 0, "control", "kill", "-C", project, "--env", "staging", "--json", "plugin", "loans")

	devices, poll := benchDevices(), benchPoll()
	results := []propagationResult{
		propagationRun(t, st, project, fcm, false, devices, poll, []string{"control", "unkill", "-C", project, "--env", "staging", "--json", "plugin", "loans"}),
		propagationRun(t, st, project, fcm, true, devices, poll, []string{"control", "kill", "-C", project, "--env", "staging", "--json", "plugin", "loans"}),
	}
	out, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("control propagation:\n%s", out)
	if path := os.Getenv("PLUX_BENCH_CONTROL_OUT"); path != "" {
		if err := os.WriteFile(path, out, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	poller, pushed := results[0], results[1]
	if pushed.P95Seconds > 60 {
		t.Errorf("with silent push the p95 is %.1f s, over the 60 s of NFR-024", pushed.P95Seconds)
	}
	if pushed.Pushes < int64(devices) {
		t.Errorf("%d pushes for %d devices", pushed.Pushes, devices)
	}
	if pushed.P95Seconds >= poller.P95Seconds {
		t.Errorf("push p95 %.1f s is not below polling p95 %.1f s", pushed.P95Seconds, poller.P95Seconds)
	}
	if poller.P95Seconds > poll.Seconds()*1.5+5 {
		t.Errorf("polling p95 %.1f s is far beyond the %.0f s interval", poller.P95Seconds, poll.Seconds())
	}
	// Both changes were seen by every device and recorded by the server.
	waitUntil(t, 10*time.Second, func() bool { return histogramCount(t, st) >= uint64(max(2*devices, 0)) }, fmt.Sprintf("%d propagation samples", 2*devices))
}

// Verifies: DEP-051, NFR-023.
// A few synthetic devices run against the real server for a few seconds at
// a hundredfold pacing: every one registers, checks the manifest, fetches
// the control document, sends telemetry, and no request fails.
func TestLoadGeneratorAgainstTheServer_DEP_051(t *testing.T) {
	st := startStackWith(t, "127.0.0.1:0", stackOptions{serverYAML: "  trustedProxies: [127.0.0.1/32]\n"})
	project := st.project(t, "loan-calculator")
	delegateControl(t, st, "staging")
	st.run(t, 0, "publish", "-C", project, "--promote", "staging", "--json")
	st.run(t, 0, "control", "kill", "-C", project, "--env", "staging", "--json", "plugin", "loans")

	fleet, err := loadgen.NewFleet(loadgen.Config{
		BaseURL: st.server, AppID: st.app.ID, Environment: "staging", Devices: 4, Duration: 3 * time.Second,
		ForwardedFor: true, Seed: 7, Scale: 100, Pacing: loadgen.DefaultPacing().Scaled(100),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fleet.Register(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(t.Context(), 3*time.Second)
	defer stop()
	report := fleet.Run(ctx)
	if report.Registered != 4 || report.Errors() != 0 {
		t.Fatalf("registered %d of 4, %d errors: %+v", report.Registered, report.Errors(), report.Operations)
	}
	for _, op := range []string{loadgen.OpManifest, loadgen.OpControl, loadgen.OpTelemetry} {
		if report.Operations[op].Requests == 0 {
			t.Errorf("no %s request in %+v", op, report.Operations)
		}
	}
	if report.ControlApplied < 4 {
		t.Errorf("%d control_applied events from 4 devices", report.ControlApplied)
	}
}
