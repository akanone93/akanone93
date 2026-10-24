// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// fakeDev is DevSessionService with scripted events and answers.
type fakeDev struct {
	pluxv1connect.UnimplementedDevSessionServiceHandler
	mu sync.Mutex
	// events are what WatchEvents streams, in order; with follow it then
	// stays open until the client leaves.
	events []*pluxv1.DevEvent
	follow bool
	// pairingExpiry is when the first pairing token stops working.
	pairingExpiry time.Time
	// status is how the devices answer a command.
	status pluxv1.DevCommandStatus
	trace  []byte

	created  []*pluxv1.DevSessionServiceCreateSessionRequest
	pairings int
	pushes   []*pluxv1.DevSessionServicePushPatchRequest
	commands []*pluxv1.DevSessionServiceSendCommandRequest
	watches  []*pluxv1.DevSessionServiceWatchEventsRequest
	exports  []*pluxv1.DevSessionServiceExportTraceRequest
	ended    []string
}

var pngBytes = []byte("\x89PNG\r\n\x1a\nfake")

func (f *fakeDev) CreateSession(_ context.Context, r *connect.Request[pluxv1.DevSessionServiceCreateSessionRequest]) (*connect.Response[pluxv1.DevSessionServiceCreateSessionResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, r.Msg)
	expiry := f.pairingExpiry
	if expiry.IsZero() {
		expiry = time.Now().Add(time.Hour)
	}
	return connect.NewResponse(&pluxv1.DevSessionServiceCreateSessionResponse{
		Session: &pluxv1.DevSession{Id: "s1", AppId: r.Msg.GetAppId(), EnvironmentId: r.Msg.GetEnvironmentId(), ExpiresAt: timestamppb.New(time.Date(2026, 10, 23, 18, 0, 0, 0, time.UTC))},
		Pairing: &pluxv1.DevPairing{Token: "T1", Uri: "plux-dev://pair?server=x&token=T1", Code: "7KQ2-M9XD", ExpiresAt: timestamppb.New(expiry)},
	}), nil
}

func (f *fakeDev) GetPairing(context.Context, *connect.Request[pluxv1.DevSessionServiceGetPairingRequest]) (*connect.Response[pluxv1.DevSessionServiceGetPairingResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pairings++
	return connect.NewResponse(&pluxv1.DevSessionServiceGetPairingResponse{
		Pairing: &pluxv1.DevPairing{Token: "T2", Uri: "plux-dev://pair?server=x&token=T2", Code: "ABCD-EFGH", ExpiresAt: timestamppb.New(time.Now().Add(time.Hour))},
	}), nil
}

func (*fakeDev) ListDevices(context.Context, *connect.Request[pluxv1.DevSessionServiceListDevicesRequest]) (*connect.Response[pluxv1.DevSessionServiceListDevicesResponse], error) {
	return connect.NewResponse(&pluxv1.DevSessionServiceListDevicesResponse{}), nil
}

func (f *fakeDev) EndSession(_ context.Context, r *connect.Request[pluxv1.DevSessionServiceEndSessionRequest]) (*connect.Response[pluxv1.DevSessionServiceEndSessionResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ended = append(f.ended, r.Msg.GetSessionId())
	return connect.NewResponse(&pluxv1.DevSessionServiceEndSessionResponse{}), nil
}

func (f *fakeDev) PushPatch(_ context.Context, r *connect.Request[pluxv1.DevSessionServicePushPatchRequest]) (*connect.Response[pluxv1.DevSessionServicePushPatchResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pushes = append(f.pushes, r.Msg)
	return connect.NewResponse(&pluxv1.DevSessionServicePushPatchResponse{
		BundleSha256: strings.Repeat("ab", 32),
		Results: []*pluxv1.DevPatchResult{
			{DeviceId: "d1", Status: pluxv1.DevPatchStatus_DEV_PATCH_STATUS_APPLIED, PagesApplied: []string{"home"}},
			{DeviceId: "d2", Status: pluxv1.DevPatchStatus_DEV_PATCH_STATUS_OFFLINE},
		},
	}), nil
}

func (f *fakeDev) SendCommand(_ context.Context, r *connect.Request[pluxv1.DevSessionServiceSendCommandRequest]) (*connect.Response[pluxv1.DevSessionServiceSendCommandResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, r.Msg)
	id := "c-" + strconv.Itoa(len(f.commands))
	devices := []string{"d1"}
	if r.Msg.GetDeviceId() != "" {
		devices = []string{r.Msg.GetDeviceId()}
	}
	status := f.status
	if status == pluxv1.DevCommandStatus_DEV_COMMAND_STATUS_UNSPECIFIED {
		status = pluxv1.DevCommandStatus_DEV_COMMAND_STATUS_OK
	}
	for _, d := range devices {
		res := &pluxv1.CommandResultEvent{CommandId: id, Status: status}
		if r.Msg.GetEvaluatePxl() != nil {
			res.Result = []byte(`{"value":42}`)
		}
		if status == pluxv1.DevCommandStatus_DEV_COMMAND_STATUS_REFUSED {
			res.Error = &pluxv1.DevError{Code: "PLX-9206", Reason: "DEV_COMMAND_NOT_ALLOWED", Message: "the path is sensitive"}
		}
		f.events = append(f.events, &pluxv1.DevEvent{Sequence: uint64(len(f.events) + 1), DeviceId: d, Payload: &pluxv1.DevEvent_CommandResult{CommandResult: res}})
		if r.Msg.GetCaptureScreenshot() != nil {
			f.events = append(f.events, &pluxv1.DevEvent{Sequence: uint64(len(f.events) + 1), DeviceId: d, Payload: &pluxv1.DevEvent_Screenshot{
				Screenshot: &pluxv1.ScreenshotEvent{CommandId: id, Png: pngBytes, Width: 390, Height: 844},
			}})
		}
	}
	return connect.NewResponse(&pluxv1.DevSessionServiceSendCommandResponse{CommandId: id, DeviceIds: devices}), nil
}

func (f *fakeDev) WatchEvents(ctx context.Context, r *connect.Request[pluxv1.DevSessionServiceWatchEventsRequest], s *connect.ServerStream[pluxv1.DevSessionServiceWatchEventsResponse]) error {
	f.mu.Lock()
	f.watches = append(f.watches, r.Msg)
	f.mu.Unlock()
	for next := 0; ; {
		f.mu.Lock()
		var batch []*pluxv1.DevEvent
		for ; next < len(f.events); next++ {
			ev := f.events[next]
			if ev.GetSequence() <= r.Msg.GetAfterSequence() || (len(r.Msg.GetDeviceIds()) > 0 && !slices.Contains(r.Msg.GetDeviceIds(), ev.GetDeviceId())) {
				continue
			}
			batch = append(batch, ev)
		}
		follow := f.follow
		f.mu.Unlock()
		for _, ev := range batch {
			if err := s.Send(&pluxv1.DevSessionServiceWatchEventsResponse{Event: ev}); err != nil {
				return err //nolint:wrapcheck // the client left
			}
		}
		if !follow {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func (f *fakeDev) ExportTrace(_ context.Context, r *connect.Request[pluxv1.DevSessionServiceExportTraceRequest]) (*connect.Response[pluxv1.DevSessionServiceExportTraceResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exports = append(f.exports, r.Msg)
	return connect.NewResponse(&pluxv1.DevSessionServiceExportTraceResponse{Trace: f.trace, EventCount: 3}), nil
}

// lockedBuffer is a buffer that a running command and a test may use together.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *lockedBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *lockedBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *lockedBuffer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b.Reset()
}

// devRig runs plux dev commands against a fake server.
type devRig struct {
	t        *testing.T
	f        *fakeDev
	srv      *httptest.Server
	project  string
	out, err lockedBuffer
	ctx      context.Context
}

func newDevRig(t *testing.T) *devRig {
	t.Helper()
	t.Setenv(tokenEnv, "plux_pat_dev")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	r := &devRig{t: t, f: &fakeDev{}, project: t.TempDir(), ctx: context.Background()}
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewAppServiceHandler(fakeAppEnvs{}))
	mux.Handle(pluxv1connect.NewDevSessionServiceHandler(r.f))
	r.srv = httptest.NewServer(mux)
	t.Cleanup(r.srv.Close)
	return r
}

func (r *devRig) env() env {
	return env{
		stdout: &r.out, stderr: &r.err, stdin: strings.NewReader(""), creds: credentials{dir: r.t.TempDir()}, httpClient: r.srv.Client(),
		pollInterval: time.Millisecond, ctx: r.ctx,
	}
}

// run runs plux dev with the server, app, project and session flags added.
func (r *devRig) run(args ...string) int {
	r.t.Helper()
	r.out.Reset()
	r.err.Reset()
	flags := []string{"--server", r.srv.URL, "--app", "a1", "-C", r.project, "--session", "s1"}
	head := 1
	if args[0] == "trace" {
		head = 2
	}
	return r.env().dev(append(append(slices.Clone(args[:head]), flags...), args[head:]...))
}

// runPlain runs plux dev with exactly these arguments.
func (r *devRig) runPlain(args ...string) int {
	r.t.Helper()
	r.out.Reset()
	r.err.Reset()
	return r.env().dev(args)
}

func compactJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// eventually waits for cond.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func writeDoc(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Verifies: DEV-041, DEV-002, DEV-010.
// plux dev creates a session, prints the pairing link, refreshes the token
// before it expires while no device is paired, pushes a saved document as a
// whole-document patch (not the dot-directories or build outputs), prints the
// result of each device and the hot-apply timing, and ends the session when
// the context ends, as Ctrl-C does.
func TestDevSession_DEV_041(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	r := newDevRig(t)
	r.f.follow = true
	r.f.pairingExpiry = time.Now().Add(time.Second)
	r.f.events = []*pluxv1.DevEvent{{Sequence: 1, DeviceId: "d1", TimeUnixMicros: 1_000_000, Payload: &pluxv1.DevEvent_HotApply{
		HotApply: &pluxv1.HotApplyEvent{BundleSha256: strings.Repeat("ab", 32), PagesApplied: []string{"home"}, ApplyMicros: 42_000},
	}}}
	writeDoc(t, r.project, "plugins/shop/pages/home.page.json", `{"kind":"page"}`)
	ctx, cancel := context.WithCancel(context.Background())
	r.ctx = ctx
	done := make(chan int, 1)
	go func() {
		done <- r.runPlain("--server", r.srv.URL, "--app", "a1", "--dir", r.project, "--env", "development")
	}()
	eventually(t, "the pairing link", func() bool {
		return strings.Contains(r.out.String(), "pairing link: plux-dev://pair?server=x&token=T1")
	})
	eventually(t, "the pairing code", func() bool { return strings.Contains(r.out.String(), "Code: 7KQ2-M9XD (expires in 0:0") })
	eventually(t, "a refreshed link", func() bool { return strings.Contains(r.out.String(), "token=T2") })
	eventually(t, "a refreshed code", func() bool { return strings.Contains(r.out.String(), "Code: ABCD-EFGH (expires in ") })
	eventually(t, "the hot apply", func() bool { return strings.Contains(r.out.String(), "hot_apply bundle abababababab applied in 42ms") })
	sessionFile := filepath.Join(r.project, devSessionFile)
	if data, err := os.ReadFile(sessionFile); err != nil || strings.TrimSpace(string(data)) != "s1" {
		t.Errorf("session file: %q %v", data, err)
	}

	time.Sleep(3 * devPollInterval)
	changed := `{"kind":"page","title":"Home"}`
	writeDoc(t, r.project, ".plux/ignored.json", `{}`)
	writeDoc(t, r.project, "build/out.json", `{}`)
	writeDoc(t, r.project, "plugins/shop/pages/home.page.json", changed)
	eventually(t, "the push", func() bool {
		r.f.mu.Lock()
		defer r.f.mu.Unlock()
		return len(r.f.pushes) > 0
	})
	eventually(t, "the push result", func() bool { return strings.Contains(r.out.String(), "device d2: offline\n") })
	cancel()
	if code := <-done; code != exitOK {
		t.Fatalf("exit %d: %s", code, r.err.String())
	}

	r.f.mu.Lock()
	defer r.f.mu.Unlock()
	if len(r.f.created) != 1 || r.f.created[0].GetAppId() != "a1" || r.f.created[0].GetEnvironmentId() != "e-dev" {
		t.Errorf("created %v", r.f.created)
	}
	if len(r.f.pushes) != 1 || r.f.pushes[0].GetSessionId() != "s1" || len(r.f.pushes[0].GetPatches()) != 1 {
		t.Fatalf("pushes %v", r.f.pushes)
	}
	p := r.f.pushes[0].GetPatches()[0]
	if p.GetPath() != "plugins/shop/pages/home.page.json" || p.GetPluginId() != "shop" || len(p.GetPatch()) != 1 {
		t.Errorf("patch %v", p)
	}
	if op := p.GetPatch()[0]; op.GetOp() != "replace" || op.GetPath() != "" || string(op.GetValue()) != changed {
		t.Errorf("op %v", op)
	}
	if r.f.pairings == 0 {
		t.Error("the pairing token was not refreshed")
	}
	if !slices.Equal(r.f.ended, []string{"s1"}) {
		t.Errorf("ended %v", r.f.ended)
	}
	out := r.out.String()
	for _, want := range []string{
		"session s1 (expires 2026-10-23T18:00:00Z)", "pushed 1 document in ", "(bundle abababababab)",
		"  plugins/shop/pages/home.page.json\n", "device d1: applied, pages home\n", "device d2: offline\n", "session s1 ended\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(sessionFile); err == nil {
		t.Error("the session file outlived the session")
	}
}

// Verifies: DEV-002.
// The pairing code's lifetime is shown in minutes and seconds.
func TestDevPairingCountdown_DEV_002(t *testing.T) {
	t.Parallel()
	for d, want := range map[time.Duration]string{
		2 * time.Minute: "2:00", 119*time.Second + 600*time.Millisecond: "2:00", 5 * time.Second: "0:05", -time.Second: "0:00",
	} {
		if got := countdown(d); got != want {
			t.Errorf("countdown(%s) = %q, want %q", d, got, want)
		}
	}
}

func TestDevSessionUsage_DEV_041(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	r := newDevRig(t)
	if code := r.runPlain("bogus"); code != exitUsage {
		t.Errorf("an unknown command: %d", code)
	}
	if code := r.runPlain("--server", r.srv.URL, "--app", "a1", "--json"); code != exitUsage {
		t.Errorf("--json on the session: %d", code)
	}
	if code := r.runPlain("logs", "--server", r.srv.URL, "-C", r.project); code != exitUsage {
		t.Errorf("no session: %d %s", code, r.err.String())
	}
}

// devEvents is a scripted stream of every kind plux dev logs renders.
func devEvents() []*pluxv1.DevEvent {
	return []*pluxv1.DevEvent{
		{Sequence: 1, DeviceId: "d1", TimeUnixMicros: 1_000_000, Payload: &pluxv1.DevEvent_Log{Log: &pluxv1.LogEvent{Level: pluxv1.DevLogLevel_DEV_LOG_LEVEL_INFO, Source: "runtime", Message: "started"}}},
		{Sequence: 2, DeviceId: "d1", TimeUnixMicros: 2_000_000, TraceId: "t1", Payload: &pluxv1.DevEvent_ActionTrace{ActionTrace: &pluxv1.ActionTraceEvent{
			RunId: "r1", Graph: "checkout", Trigger: "tap", NodePath: "pages/cart/button[2]", Status: pluxv1.DevRunStatus_DEV_RUN_STATUS_FAILED,
			StartUnixMicros: 2_000_000, EndUnixMicros: 2_012_000,
			Steps: []*pluxv1.ActionStepTrace{
				{StepId: "s1", Action: "validate", Branch: "main", Status: pluxv1.DevRunStatus_DEV_RUN_STATUS_SUCCEEDED, DurationMicros: 2000},
				{StepId: "s2", Action: "call_function", Branch: "onError", Status: pluxv1.DevRunStatus_DEV_RUN_STATUS_FAILED, DurationMicros: 9000, Error: &pluxv1.DevError{Code: "PLX-4100", Message: "boom"}},
			},
		}}},
		{Sequence: 3, DeviceId: "d2", TimeUnixMicros: 3_000_000, TraceId: "t1", Payload: &pluxv1.DevEvent_FunctionCall{FunctionCall: &pluxv1.FunctionCallEvent{
			Function: "bank.fee", Version: 4, Placement: "server", DurationMicros: 1500,
			Logs: []*pluxv1.DevLogLine{{Level: pluxv1.DevLogLevel_DEV_LOG_LEVEL_WARNING, Message: "rounding applied"}},
		}}},
		{Sequence: 4, DeviceId: "d2", TimeUnixMicros: 4_000_000, Payload: &pluxv1.DevEvent_NetworkCall{NetworkCall: &pluxv1.NetworkCallEvent{
			Method: "GET", UrlTemplate: "/accounts/{id}", Status: 200, TotalMicros: 80_000,
		}}},
	}
}

const devActionTraceLines = "2 00:00:02.000 d1 action_trace checkout tap on pages/cart/button[2] failed 12ms\n" +
	"  s1 validate [main] succeeded 2ms\n" +
	"    s2 call_function [onError] failed 9ms PLX-4100 boom\n"

// Verifies: DEV-021, DEV-041.
// plux dev logs prints one line per event, an action run as an indented step
// tree and a function call with its merged log lines; --filter keeps one
// payload kind or a text, --device and --since go to the server, and --json
// prints one line of proto3 JSON per event.
func TestDevLogs_DEV_021(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	r := newDevRig(t)
	r.f.events = devEvents()
	want := "1 00:00:01.000 d1 log info runtime: started\n" + devActionTraceLines +
		"3 00:00:03.000 d2 function_call bank.fee@v4 server 1.5ms\n  | warning rounding applied\n" +
		"4 00:00:04.000 d2 network_call GET /accounts/{id} 200 80ms\n"
	if code := r.run("logs"); code != exitOK || r.out.String() != want {
		t.Fatalf("logs: %d %s\n%s", code, r.err.String(), r.out.String())
	}
	if code := r.run("logs", "--filter", "action_trace"); code != exitOK || r.out.String() != devActionTraceLines {
		t.Errorf("--filter kind: %d\n%s", code, r.out.String())
	}
	if code := r.run("logs", "--filter", "ROUNDING"); code != exitOK || !strings.HasPrefix(r.out.String(), "3 00:00:03.000 d2 function_call") || strings.Count(r.out.String(), "\n") != 2 {
		t.Errorf("--filter text: %d\n%s", code, r.out.String())
	}
	if code := r.run("logs", "--since", "3", "--device", "d2"); code != exitOK || r.out.String() != "4 00:00:04.000 d2 network_call GET /accounts/{id} 200 80ms\n" {
		t.Errorf("--since: %d\n%s", code, r.out.String())
	}
	if w := r.f.watches[len(r.f.watches)-1]; w.GetSessionId() != "s1" || w.GetAfterSequence() != 3 || !slices.Equal(w.GetDeviceIds(), []string{"d2"}) {
		t.Errorf("watch request %v", w)
	}
	if code := r.run("logs", "--json"); code != exitOK {
		t.Fatalf("--json: %d %s", code, r.err.String())
	}
	lines := strings.Split(strings.TrimSuffix(r.out.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("--json lines: %d\n%s", len(lines), r.out.String())
	}
	for i, l := range lines {
		var ev pluxv1.DevEvent
		if err := protojson.Unmarshal([]byte(l), &ev); err != nil || ev.GetSequence() != uint64(i+1) || strings.Contains(l, "\n") {
			t.Errorf("line %d: %v %s", i, err, l)
		}
	}
	if strings.Contains(lines[0], ": ") {
		t.Errorf("--json is not compact: %s", lines[0])
	}
}

// buildTrace writes a trace file the way the server does.
func buildTrace(t *testing.T, version uint32, events []*pluxv1.DevEvent) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	header := &pluxv1.TraceFileHeader{
		FormatVersion: version, SessionId: "s1", AppId: "a1", AppKey: "shop", EnvironmentId: "e-dev", EnvironmentKey: "development",
		ExportedAt: timestamppb.New(time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)), ServerVersion: "0.9.0",
	}
	if _, err := protodelim.MarshalTo(zw, header); err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if _, err := protodelim.MarshalTo(zw, ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Verifies: DEV-021.
// plux dev trace export writes the server's bytes to a file, and plux dev
// trace show reads that file offline and prints it like plux dev logs; a file
// that is not a trace, is cut short or is of a newer format fails with
// PLX-9233.
func TestDevTrace_DEV_021(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	r := newDevRig(t)
	r.f.events = devEvents()
	r.f.trace = buildTrace(t, 1, devEvents())
	file := filepath.Join(t.TempDir(), "bug.trace")
	if code := r.run("trace", "export", "--out", file, "--device", "d1"); code != exitOK {
		t.Fatalf("export: %d %s", code, r.err.String())
	}
	if got, _ := os.ReadFile(file); !bytes.Equal(got, r.f.trace) { //nolint:gosec // a test file
		t.Error("the file is not what the server sent")
	}
	if e := r.f.exports[0]; e.GetSessionId() != "s1" || !slices.Equal(e.GetDeviceIds(), []string{"d1"}) {
		t.Errorf("export request %v", e)
	}
	if !strings.Contains(r.out.String(), "wrote "+file+" (3 events, ") {
		t.Errorf("export output: %s", r.out.String())
	}
	if code := r.run("trace", "export"); code != exitUsage {
		t.Errorf("export without --out: %d", code)
	}

	logs := func() string {
		if code := r.run("logs"); code != exitOK {
			t.Fatalf("logs: %d", code)
		}
		return r.out.String()
	}()
	if code := r.runPlain("trace", "show", file); code != exitOK {
		t.Fatalf("show: %d %s", code, r.err.String())
	}
	head, events, _ := strings.Cut(r.out.String(), "\n")
	if head != "trace of session s1: app shop, environment development, exported 2026-10-23T12:00:00Z by server 0.9.0, 4 events" || events != logs {
		t.Errorf("show:\n%s\nlogs:\n%s", r.out.String(), logs)
	}
	if code := r.runPlain("trace", "show", "--filter", "network_call", file); code != exitOK || !strings.HasSuffix(r.out.String(), "4 00:00:04.000 d2 network_call GET /accounts/{id} 200 80ms\n") || strings.Count(r.out.String(), "\n") != 2 {
		t.Errorf("show --filter: %d\n%s", code, r.out.String())
	}
	if code := r.runPlain("trace", "show", "--json", file); code != exitOK || strings.Count(r.out.String(), "\n") != 5 || !strings.Contains(r.out.String(), `"formatVersion":1`) {
		t.Errorf("show --json: %d\n%s", code, r.out.String())
	}

	dir := t.TempDir()
	good := buildTrace(t, 1, devEvents())
	bad := map[string][]byte{
		"garbage.trace": []byte("this is not a trace file"),
		"empty.trace":   {},
		"newer.trace":   buildTrace(t, 2, nil),
		"cut.trace":     good[:len(good)-12],
	}
	for name, data := range bad {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if code := r.runPlain("trace", "show", path); code != exitFailed || !strings.Contains(r.err.String(), "PLX-9233") || r.out.String() != "" {
			t.Errorf("%s: %d\nstdout %q\nstderr %s", name, code, r.out.String(), r.err.String())
		}
	}
	if code := r.runPlain("trace", "show", filepath.Join(dir, "missing.trace")); code != exitFailed || !strings.Contains(r.err.String(), "PLX-9233") {
		t.Errorf("missing file: %d %s", code, r.err.String())
	}
}

// Verifies: DEV-011, DEV-031.
// Every plux dev command sends its command, prints the command identifier and
// the answer of the device, and fails when the device does not say ok.
func TestDevCommands_DEV_011(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	r := newDevRig(t)
	r.f.follow = true
	last := func() *pluxv1.DevSessionServiceSendCommandRequest {
		r.f.mu.Lock()
		defer r.f.mu.Unlock()
		return r.f.commands[len(r.f.commands)-1]
	}
	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, c *pluxv1.DevSessionServiceSendCommandRequest)
	}{
		{"open", []string{"open", "/home", "--param", "id=7", "--param", "tab=a", "--locale", "de", "--theme", "dark", "--text-scale", "1.5", "--mock", "accounts=empty"}, func(t *testing.T, c *pluxv1.DevSessionServiceSendCommandRequest) {
			o := c.GetOpenScreen()
			if o.GetRoute() != "/home" || o.GetParams()["id"] != "7" || o.GetParams()["tab"] != "a" || o.GetLocale() != "de" || o.GetTheme() != pluxv1.DevThemeMode_DEV_THEME_MODE_DARK ||
				o.GetTextScale() != 1.5 || len(o.GetMocks()) != 1 || o.GetMocks()[0].GetSource() != "accounts" || o.GetMocks()[0].GetState() != "empty" {
				t.Errorf("open %v", o)
			}
		}},
		{"set", []string{"set", "page:/home", "form.name", `"Ada"`}, func(t *testing.T, c *pluxv1.DevSessionServiceSendCommandRequest) {
			if s := c.GetSetState(); s.GetScope() != "page:/home" || s.GetPath() != "form.name" || string(s.GetValue()) != `"Ada"` {
				t.Errorf("set %v", s)
			}
		}},
		{"mock", []string{"mock", "accounts", "error"}, func(t *testing.T, c *pluxv1.DevSessionServiceSendCommandRequest) {
			if m := c.GetSwitchMock(); m.GetSource() != "accounts" || m.GetState() != "error" {
				t.Errorf("mock %v", m)
			}
		}},
		{"clock set", []string{"clock", "set", "2026-10-23T10:00:00Z"}, func(t *testing.T, c *pluxv1.DevSessionServiceSendCommandRequest) {
			if c.GetFakeClock().GetSetUnixMicros() != time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC).UnixMicro() {
				t.Errorf("clock %v", c.GetFakeClock())
			}
		}},
		{"clock advance", []string{"clock", "advance", "90m"}, func(t *testing.T, c *pluxv1.DevSessionServiceSendCommandRequest) {
			if c.GetFakeClock().GetAdvanceMicros() != (90 * time.Minute).Microseconds() {
				t.Errorf("clock %v", c.GetFakeClock())
			}
		}},
		{"clock reset", []string{"clock", "reset"}, func(t *testing.T, c *pluxv1.DevSessionServiceSendCommandRequest) {
			if !c.GetFakeClock().GetReset_() {
				t.Errorf("clock %v", c.GetFakeClock())
			}
		}},
		{"lifecycle", []string{"lifecycle", "memory-pressure"}, func(t *testing.T, c *pluxv1.DevSessionServiceSendCommandRequest) {
			if c.GetLifecycle().GetEvent() != pluxv1.DevLifecycleEvent_DEV_LIFECYCLE_EVENT_MEMORY_PRESSURE {
				t.Errorf("lifecycle %v", c.GetLifecycle())
			}
		}},
		{"network", []string{"network", "3g"}, func(t *testing.T, c *pluxv1.DevSessionServiceSendCommandRequest) {
			if c.GetNetworkProfile().GetProfileId() != "3g" {
				t.Errorf("network %v", c.GetNetworkProfile())
			}
		}},
		{"security-detect", []string{"security-detect", "untrusted-installer"}, func(t *testing.T, c *pluxv1.DevSessionServiceSendCommandRequest) {
			if c.GetSimulateSecurityDetection().GetKind() != pluxv1.DevSecurityDetectionKind_DEV_SECURITY_DETECTION_KIND_UNTRUSTED_INSTALLER {
				t.Errorf("detection %v", c.GetSimulateSecurityDetection())
			}
		}},
		{"pxl", []string{"pxl", "state.count + 1"}, func(t *testing.T, c *pluxv1.DevSessionServiceSendCommandRequest) {
			if c.GetEvaluatePxl().GetExpression() != "state.count + 1" {
				t.Errorf("pxl %v", c.GetEvaluatePxl())
			}
		}},
		{"inspect", []string{"inspect", "pages/cart/button[2]"}, func(t *testing.T, c *pluxv1.DevSessionServiceSendCommandRequest) {
			if c.GetInspectSelect().GetNodePath() != "pages/cart/button[2]" {
				t.Errorf("inspect %v", c.GetInspectSelect())
			}
		}},
		{"overlay", []string{"overlay", "frame-chart", "on"}, func(t *testing.T, c *pluxv1.DevSessionServiceSendCommandRequest) {
			if o := c.GetToggleOverlay(); o.GetOverlay() != pluxv1.DevOverlay_DEV_OVERLAY_FRAME_CHART || !o.GetEnabled() {
				t.Errorf("overlay %v", o)
			}
		}},
		{"overlay off", []string{"overlay", "contrast", "off"}, func(t *testing.T, c *pluxv1.DevSessionServiceSendCommandRequest) {
			if o := c.GetToggleOverlay(); o.GetOverlay() != pluxv1.DevOverlay_DEV_OVERLAY_CONTRAST || o.GetEnabled() {
				t.Errorf("overlay %v", o)
			}
		}},
	}
	for i, tc := range tests {
		args := append(slices.Clone(tc.args), "--device", "d1")
		if code := r.run(args...); code != exitOK {
			t.Fatalf("%s: %d %s", tc.name, code, r.err.String())
		}
		id := "c-" + strconv.Itoa(i+1)
		if !strings.HasPrefix(r.out.String(), "command "+id+" sent to d1\nd1: ok") {
			t.Errorf("%s output:\n%s", tc.name, r.out.String())
		}
		c := last()
		if c.GetSessionId() != "s1" || c.GetDeviceId() != "d1" {
			t.Errorf("%s: target %v", tc.name, c)
		}
		tc.check(t, c)
	}

	if code := r.run("pxl", "x"); code != exitOK || r.out.String() != "command c-14 sent to d1\nd1: ok {\"value\":42}\n" {
		t.Errorf("pxl result: %d\n%s", code, r.out.String())
	}
	if c := last(); c.GetDeviceId() != "" {
		t.Errorf("without --device the command goes to every device: %v", c)
	}
	if code := r.run("pxl", "x", "--json"); code != exitOK {
		t.Fatalf("--json: %d", code)
	}
	var got struct {
		CommandID string `json:"commandId"`
		Results   []struct {
			Device, Status string
			Result         json.RawMessage
		}
	}
	if err := json.Unmarshal([]byte(r.out.String()), &got); err != nil || got.CommandID != "c-15" || len(got.Results) != 1 || got.Results[0].Status != "ok" || compactJSON(t, got.Results[0].Result) != `{"value":42}` {
		t.Errorf("--json: %v %s", err, r.out.String())
	}

	for _, args := range [][]string{
		{"network", "dialup"},
		{"lifecycle", "nap"},
		{"security-detect", "ghost"},
		{"overlay", "frame-chart", "maybe"},
		{"overlay", "glow", "on"},
		{"clock", "set", "noon"},
		{"clock", "advance", "-5m"},
		{"clock"},
		{"set", "app", "x", "{bad"},
		{"open", "/h", "--param", "novalue"},
		{"open", "/h", "--theme", "sepia"},
		{"mock", "only-one"},
		{"screenshot"},
	} {
		before := len(r.f.commands)
		if code := r.run(args...); code != exitUsage || len(r.f.commands) != before {
			t.Errorf("%v: %d (commands sent: %d)", args, code, len(r.f.commands)-before)
		}
	}
}

// Verifies: DEV-011.
// A device that refuses a command makes plux dev fail and show its PLX code.
func TestDevCommandRefused_DEV_011(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	r := newDevRig(t)
	r.f.follow = true
	r.f.status = pluxv1.DevCommandStatus_DEV_COMMAND_STATUS_REFUSED
	if code := r.run("set", "app", "card", `"4111"`); code != exitFailed {
		t.Errorf("exit %d", code)
	}
	if want := "command c-1 sent to d1\nd1: refused PLX-9206 the path is sensitive\n"; r.out.String() != want {
		t.Errorf("output:\n%s", r.out.String())
	}
}

// Verifies: DEV-033.
// plux dev screenshot waits for the image and writes the PNG.
func TestDevScreenshot_DEV_033(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	r := newDevRig(t)
	r.f.follow = true
	out := filepath.Join(t.TempDir(), "shot.png")
	if code := r.run("screenshot", "--out", out); code != exitOK {
		t.Fatalf("exit %d %s", code, r.err.String())
	}
	if got, _ := os.ReadFile(out); !bytes.Equal(got, pngBytes) { //nolint:gosec // a test file
		t.Errorf("file %q", got)
	}
	if want := "command c-1 sent to d1\nd1: ok\nd1: wrote " + out + " (390x844)\n"; r.out.String() != want {
		t.Errorf("output:\n%s", r.out.String())
	}
	if shotPath("a/shot.png", "d2", 2) != "a/shot-d2.png" || shotPath("a/shot.png", "d2", 1) != "a/shot.png" {
		t.Error("shotPath")
	}
}

// Verifies: DEV-041.
// The commands find the session plux dev recorded in the project.
func TestDevCommandsUseRecordedSession_DEV_041(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	r := newDevRig(t)
	r.f.follow = true
	writeDoc(t, r.project, devSessionFile, "s9\n")
	if code := r.runPlain("lifecycle", "--server", r.srv.URL, "-C", r.project, "pause"); code != exitOK {
		t.Fatalf("exit %d %s", code, r.err.String())
	}
	if got := r.f.commands[0].GetSessionId(); got != "s9" {
		t.Errorf("session %q", got)
	}
}
