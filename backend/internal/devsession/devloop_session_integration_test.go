// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/api"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/dpop"
	"github.com/nightCode42/plux3/backend/internal/icons"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// session is the scripted debugging session's world: the dev-session
// service behind the same ConnectRPC handlers and device socket the server
// serves, and the project the developer edits.
type session struct {
	*relay
	project string
	plux    string // the built CLI
}

// lockedBuffer is the output of a CLI process, read while it runs.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// appHandler answers the two calls the CLI makes to resolve the app and its
// environment from the world.
type appHandler struct {
	pluxv1connect.UnimplementedAppServiceHandler
	f *fixture
}

func (h appHandler) ListApps(context.Context, *connect.Request[pluxv1.ListAppsRequest]) (*connect.Response[pluxv1.ListAppsResponse], error) {
	return connect.NewResponse(&pluxv1.ListAppsResponse{Apps: []*pluxv1.App{{Id: h.f.App, Key: "loans"}}}), nil
}

func (h appHandler) ListEnvironments(context.Context, *connect.Request[pluxv1.ListEnvironmentsRequest]) (*connect.Response[pluxv1.ListEnvironmentsResponse], error) {
	var envs []*pluxv1.Environment
	for key, id := range h.f.Envs {
		envs = append(envs, &pluxv1.Environment{Id: id, Key: key})
	}
	return connect.NewResponse(&pluxv1.ListEnvironmentsResponse{Environments: envs}), nil
}

// newSession serves the world and builds the CLI.
func newSession(t *testing.T) *session {
	t.Helper()
	f := newFixture(t, 0)
	svc, err := devsession.New(devsession.Options{
		DB: f.DB, Audit: f.Log, IDs: f.Gen, Tokens: f.signer, Environments: []string{"development"},
		PublicBaseURL: "https://plux.example.com", Now: f.Clock,
		Limits: func(ctx context.Context, tx pgx.Tx, org, _ string) (limits.Set, error) { return f.Limits(ctx, tx, org) },
		Drafts: drafts{t}, Signer: fileSigner{f},
		CompileOptions: func(context.Context, pgx.Tx, string, string, string) (compiler.Options, error) {
			o := compiler.DefaultOptions()
			o.IconFont = func(set icons.Set, names []string) ([]byte, error) {
				return fmt.Appendf(nil, "font %s %v", set, names), nil
			}
			return o, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.svc = svc
	r := &relay{fixture: f, used: map[string]bool{}}

	// The developer is signed in whatever token the CLI sends; signing in
	// is tested with the server's authentication.
	developer := api.Around(func(ctx context.Context, _ api.Call, next func(context.Context) error) error {
		return next(api.WithIdentity(ctx, f.Developer.Identity))
	})
	h := &api.Handlers{DevSessions: svc, Auth: f.Auth, Limits: limits.Defaults()}
	opts := connect.WithInterceptors(api.Interceptors(api.Deps{Before: []api.Around{developer}})...)
	mux := http.NewServeMux()
	for _, reg := range []func() (string, http.Handler){
		func() (string, http.Handler) { return pluxv1connect.NewDevSessionServiceHandler(h.DevSession(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewAppServiceHandler(appHandler{f: f}, opts) },
	} {
		path, handler := reg()
		mux.Handle(path, handler)
	}
	mux.Handle("GET "+devsession.SocketPath, svc.SocketHandler(r.proof))
	r.srv = httptest.NewServer(mux)
	t.Cleanup(r.srv.Close)

	// The project is a copy of the conformance project: the developer's
	// working directory.
	project := t.TempDir()
	root := os.DirFS(fixtureDir)
	err = fs.WalkDir(root, ".", func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		data, err := fs.ReadFile(root, p)
		if err != nil {
			return err //nolint:wrapcheck // a test
		}
		dst := filepath.Join(project, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return err //nolint:wrapcheck // a test
		}
		return os.WriteFile(dst, data, 0o600) //nolint:wrapcheck // a test
	})
	if err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(t.TempDir(), "plux")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, "../../cmd/plux")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the CLI: %v\n%s", err, out)
	}
	return &session{relay: r, project: project, plux: bin}
}

// command prepares the CLI for the server of the world, signed in with a
// token of its own and a configuration directory of its own.
func (s *session) command(t *testing.T, args ...string) (*exec.Cmd, *lockedBuffer) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), s.plux, args...) //nolint:gosec // G204: the CLI the test built
	cmd.Dir = s.project
	cmd.Env = append(os.Environ(), "PLUX_TOKEN=plux_pat_devloop", "PLUX_SERVER="+s.srv.URL, "PLUX_ORGANIZATION="+s.Org,
		"HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	out := &lockedBuffer{}
	cmd.Stdout, cmd.Stderr = out, out
	return cmd, out
}

// until waits for the output to contain every text.
func until(t *testing.T, out *lockedBuffer, texts ...string) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		got := out.String()
		if !slices.ContainsFunc(texts, func(w string) bool { return !strings.Contains(got, w) }) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the output lacks %q:\n%s", texts, out.String())
}

// interrupt ends a CLI process as Ctrl-C does and waits for it.
func interrupt(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("the CLI did not end cleanly: %v", err)
	}
}

// watching runs a plux dev command that watches until interrupted, and
// returns its output once it holds every text.
func (s *session) watching(t *testing.T, texts []string, args ...string) string {
	t.Helper()
	cmd, out := s.command(t, args...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	until(t, out, texts...)
	interrupt(t, cmd)
	return out.String()
}

// run runs a plux dev command that ends by itself.
func (s *session) run(t *testing.T, args ...string) string {
	t.Helper()
	cmd, out := s.command(t, args...)
	if err := cmd.Run(); err != nil {
		t.Fatalf("plux %v: %v\n%s", args, err, out)
	}
	return out.String()
}

// blocks splits CLI output into its events: a line that starts with a
// sequence number, and the indented lines that follow it.
func blocks(output string) []string {
	var out []string
	for _, line := range strings.Split(output, "\n") {
		switch {
		case line == "":
		case line[0] >= '0' && line[0] <= '9':
			out = append(out, line)
		case len(out) > 0 && line[0] == ' ':
			out[len(out)-1] += "\n" + line
		}
	}
	return out
}

// pageWithPersistence is the page of the project with the session
// persistence the developer adds.
func pageWithPersistence(t *testing.T, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(calculator))) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	var page map[string]any
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatal(err)
	}
	state, ok := page["state"].([]any)
	if !ok || len(state) < 2 {
		t.Fatal("the page has no second state entry")
	}
	state[1].(map[string]any)["persistence"] = "session" //nolint:forcetypeassert // the project's own shape
	out, err := json.MarshalIndent(page, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Verifies: DEV-041, DEV-021, DEV-020, FN-036.
// A developer with no Studio debugs a flow with the CLI alone (exit
// criterion X2). plux dev opens a session on the server and prints its
// pairing link; a simulated device pairs with the link's token and opens
// the real device socket; saving an edit delivers the compiled bundle to the
// device. The device then reports a failed action run with its node path, a
// state change and a server function call whose log lines the server
// stored; plux dev logs shows the trace with its node path, the state diff
// and the call merged with its server log line, and a trace file exported
// with plux dev trace export reads back, with plux dev trace show, as the
// same events. The emulator variant, with the real runtime, runs in CI's
// emulator job.
func TestDevLoopSession_DEV_041_DEV_021_DEV_020_FN_036(t *testing.T) {
	s := newSession(t)

	// plux dev opens the session and prints how to pair.
	dev, devOut := s.command(t, "dev", "--app", s.App, "--dir", s.project)
	if err := dev.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dev.Process.Kill() })
	until(t, devOut, "session ", "pairing link: ", "watching ")
	id := regexp.MustCompile(`session (\S+) \(expires`).FindStringSubmatch(devOut.String())
	link := regexp.MustCompile(`pairing link: (\S+)`).FindStringSubmatch(devOut.String())
	if id == nil || link == nil {
		t.Fatalf("no session or pairing link in:\n%s", devOut.String())
	}
	pairing, err := url.Parse(link[1])
	if err != nil || pairing.Scheme != "plux-dev" || pairing.Query().Get("token") == "" {
		t.Fatalf("the pairing link %q: %v", link[1], err)
	}

	// The simulated device pairs with the link's token and opens the socket.
	key, jkt := newDeviceKey(t)
	paired, err := s.pair(s.device("device-1", jkt), pairing.Query().Get("token"))
	if err != nil {
		t.Fatal(err)
	}
	k := s.connect(t, paired.Ticket, key, &pluxv1.DevDeviceHello{})
	k.c.SetReadLimit(64 << 20) // a bundle frame is larger than the default

	// The developer saves an edit; the device receives the bundle.
	edited := filepath.Join(s.project, filepath.FromSlash(calculator))
	if err := os.WriteFile(edited, pageWithPersistence(t, s.project), 0o600); err != nil {
		t.Fatal(err)
	}
	until(t, devOut, "pushed 1 document", calculator)
	var bundle *pluxv1.DevBundleFrame
	for bundle == nil {
		bundle = k.next(t).GetBundle()
	}
	if bundle.GetBundleSequence() == 0 {
		t.Errorf("the device received a bundle without a sequence: %v", bundle)
	}

	// The device reports a failed action run, a state change and a server
	// function call; the server stored the function's log lines.
	const trace = "trace-d10"
	at := pgtype.Timestamptz{Time: s.Now, Valid: true}
	err = s.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return dbgen.New(tx).InsertFunctionLogs(ctx, dbgen.InsertFunctionLogsParams{ //nolint:wrapcheck // a test
			OrganizationID: storage.MustUUID(s.Org), AppID: storage.MustUUID(s.App), EnvironmentID: storage.MustUUID(s.Envs["development"]),
			Function: "loans.score", Version: 1, InvocationID: storage.MustUUID(s.NewID(t)),
			Levels: []string{"info", "error"}, Messages: []string{"scoring started", "score service refused the request"},
			TraceID: trace, Times: []pgtype.Timestamptz{at, at},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	const nodePath = "/root/body/children/2"
	start := s.Now.UnixMicro()
	k.event(t, 1, &pluxv1.DevEvent{TraceId: trace, Payload: &pluxv1.DevEvent_ActionTrace{ActionTrace: &pluxv1.ActionTraceEvent{
		RunId: "run-1", Graph: "submit-application", Trigger: "onTap", NodePath: nodePath,
		Status: pluxv1.DevRunStatus_DEV_RUN_STATUS_FAILED, StartUnixMicros: start, EndUnixMicros: start + 4200,
		Steps: []*pluxv1.ActionStepTrace{
			{StepId: "s1", Action: "setState", Branch: "main", Status: pluxv1.DevRunStatus_DEV_RUN_STATUS_SUCCEEDED, DurationMicros: 300},
			{
				StepId: "s2", Action: "callFunction", Branch: "main", Status: pluxv1.DevRunStatus_DEV_RUN_STATUS_FAILED, DurationMicros: 3900,
				Error: &pluxv1.DevError{Code: "PLX-5003", Message: "the function failed"},
			},
			{StepId: "s3", Action: "showMessage", Branch: "onError", Status: pluxv1.DevRunStatus_DEV_RUN_STATUS_SUCCEEDED, DurationMicros: 200},
		},
		Error: &pluxv1.DevError{Code: "PLX-5003", Message: "the function failed"},
	}}})
	k.event(t, 2, &pluxv1.DevEvent{TraceId: trace, Payload: &pluxv1.DevEvent_StateDiff{StateDiff: &pluxv1.StateDiffEvent{
		Scope: "page:calculator", RunId: "run-1",
		Changes: []*pluxv1.StateChange{{Path: "status", OldValue: []byte(`"idle"`), NewValue: []byte(`"submitting"`)}},
	}}})
	k.event(t, 3, &pluxv1.DevEvent{TraceId: trace, Payload: &pluxv1.DevEvent_FunctionCall{FunctionCall: &pluxv1.FunctionCallEvent{
		Function: "loans.score", Version: 1, Placement: "server", StartUnixMicros: start, DurationMicros: 3900,
	}}})
	k.ackedUpTo(t, 3)

	// plux dev logs shows the failed run with its node path and steps.
	failed := s.watching(t, []string{"submit-application"}, "dev", "logs", "--session", id[1], "--filter", "action")
	for _, want := range []string{
		"action_trace", "submit-application onTap on " + nodePath + " failed", "PLX-5003",
		"s2 callFunction [main] failed", "  s3 showMessage [onError] succeeded",
	} {
		if !strings.Contains(failed, want) {
			t.Errorf("plux dev logs --filter action lacks %q:\n%s", want, failed)
		}
	}
	if strings.Contains(failed, "state_diff") || strings.Contains(failed, "function_call") {
		t.Errorf("--filter action shows other events:\n%s", failed)
	}

	// The state diff names the path with the value before and after.
	diff := s.watching(t, []string{"state_diff"}, "dev", "logs", "--session", id[1], "--filter", "state_diff")
	if !strings.Contains(diff, `page:calculator: status "idle" -> "submitting"`) {
		t.Errorf("plux dev logs --filter state_diff:\n%s", diff)
	}

	// The function call carries the log lines the server stored under the
	// event's trace ID.
	call := s.watching(t, []string{"function_call"}, "dev", "logs", "--session", id[1], "--filter", "function_call")
	for _, want := range []string{"loans.score@v1 server", "| info scoring started", "| error score service refused the request"} {
		if !strings.Contains(call, want) {
			t.Errorf("plux dev logs --filter function_call lacks %q:\n%s", want, call)
		}
	}

	// The trace file reads back as the same events.
	all := s.watching(t, []string{"function_call", "scoring started"}, "dev", "logs", "--session", id[1])
	file := filepath.Join(t.TempDir(), "session.trace")
	if out := s.run(t, "dev", "trace", "export", "--session", id[1], "--out", file); !strings.Contains(out, "events") {
		t.Errorf("plux dev trace export said %q", out)
	}
	shown := s.run(t, "dev", "trace", "show", file)
	live, offline := blocks(all), blocks(shown)
	if len(live) != 3 || !slices.Equal(live, offline) {
		t.Errorf("the trace file shows other events than the live stream.\nlive:\n%s\n\nfrom the file:\n%s", strings.Join(live, "\n"), strings.Join(offline, "\n"))
	}
	if filtered := s.run(t, "dev", "trace", "show", "--filter", "action", file); !strings.Contains(filtered, nodePath) || strings.Contains(filtered, "function_call") {
		t.Errorf("plux dev trace show --filter action:\n%s", filtered)
	}

	// Ctrl-C ends the session.
	interrupt(t, dev)
	if out := devOut.String(); !strings.Contains(out, "session "+id[1]+" ended") {
		t.Errorf("plux dev did not end its session:\n%s", out)
	}
}

// newDeviceKey makes the device's key and its thumbprint.
func newDeviceKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	key, _ := devicetest.NewKey(t)
	jkt, err := dpop.Thumbprint(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return key, jkt
}
