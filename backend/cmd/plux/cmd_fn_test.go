// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// fakeFunctions is FunctionAdminService with scripted builds.
type fakeFunctions struct {
	pluxv1connect.UnimplementedFunctionAdminServiceHandler
	mu sync.Mutex
	// builds are the states GetBuild answers in turn; the last repeats.
	builds    []*pluxv1.Build
	polls     int
	deployErr error
	uploaded  []byte
	upload    *pluxv1.UploadSourceRequest
	deploy    *pluxv1.DeployRequest
	alias     *pluxv1.SetAliasRequest
	logs      []*pluxv1.FunctionLogLine
	logReqs   []*pluxv1.ListLogsRequest
}

func (f *fakeFunctions) UploadSource(_ context.Context, s *connect.ClientStream[pluxv1.UploadSourceRequest]) (*connect.Response[pluxv1.UploadSourceResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for s.Receive() {
		m := s.Msg()
		if f.upload == nil {
			f.upload = m
			continue
		}
		f.uploaded = append(f.uploaded, m.GetChunk()...)
	}
	return connect.NewResponse(&pluxv1.UploadSourceResponse{SourceId: "src-1"}), s.Err()
}

func (*fakeFunctions) Build(context.Context, *connect.Request[pluxv1.BuildRequest]) (*connect.Response[pluxv1.BuildResponse], error) {
	return connect.NewResponse(&pluxv1.BuildResponse{Build: &pluxv1.Build{Id: "b-1", State: pluxv1.BuildState_BUILD_STATE_QUEUED}}), nil
}

func (f *fakeFunctions) GetBuild(context.Context, *connect.Request[pluxv1.GetBuildRequest]) (*connect.Response[pluxv1.GetBuildResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := min(f.polls, len(f.builds)-1)
	f.polls++
	return connect.NewResponse(&pluxv1.GetBuildResponse{Build: f.builds[i]}), nil
}

func (f *fakeFunctions) Deploy(_ context.Context, r *connect.Request[pluxv1.DeployRequest]) (*connect.Response[pluxv1.DeployResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deploy = r.Msg
	if f.deployErr != nil {
		return nil, f.deployErr
	}
	return connect.NewResponse(&pluxv1.DeployResponse{Version: &pluxv1.FunctionVersion{Name: "bank.fee", Version: 4, Placement: "server", ModuleSha256: strings.Repeat("ab", 32)}}), nil
}

func (f *fakeFunctions) SetAlias(_ context.Context, r *connect.Request[pluxv1.SetAliasRequest]) (*connect.Response[pluxv1.SetAliasResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alias = r.Msg
	return connect.NewResponse(&pluxv1.SetAliasResponse{Aliases: []*pluxv1.Alias{{Name: "prod", Version: r.Msg.GetAlias().GetVersion()}}}), nil
}

func (*fakeFunctions) ListVersions(_ context.Context, r *connect.Request[pluxv1.ListVersionsRequest]) (*connect.Response[pluxv1.ListVersionsResponse], error) {
	at := timestamppb.New(time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC))
	if r.Msg.GetPage().GetPageToken() == "" {
		return connect.NewResponse(&pluxv1.ListVersionsResponse{
			Versions: []*pluxv1.FunctionVersion{{Version: 2, Placement: "server", ModuleSha256: strings.Repeat("cd", 32), TestsPassed: true, CreatedAt: at}},
			Page:     &pluxv1.PageResult{NextPageToken: "more"},
		}), nil
	}
	return connect.NewResponse(&pluxv1.ListVersionsResponse{
		Versions: []*pluxv1.FunctionVersion{{Version: 1, Placement: "server", ModuleSha256: strings.Repeat("ef", 32), TestsPassed: true, CreatedAt: at}},
	}), nil
}

func (*fakeFunctions) GetFunction(context.Context, *connect.Request[pluxv1.GetFunctionRequest]) (*connect.Response[pluxv1.GetFunctionResponse], error) {
	return connect.NewResponse(&pluxv1.GetFunctionResponse{Aliases: []*pluxv1.Alias{{Name: "stable", Version: 2}, {Name: "prod", Version: 2}}}), nil
}

func (f *fakeFunctions) ListLogs(_ context.Context, r *connect.Request[pluxv1.ListLogsRequest]) (*connect.Response[pluxv1.ListLogsResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logReqs = append(f.logReqs, r.Msg)
	return connect.NewResponse(&pluxv1.ListLogsResponse{Lines: f.logs}), nil // newest first
}

func logLine(at time.Time, version uint32, level, msg string) *pluxv1.FunctionLogLine {
	return &pluxv1.FunctionLogLine{Time: timestamppb.New(at), Version: version, Level: level, Message: msg}
}

// fnRig runs plux fn commands against a fake server.
type fnRig struct {
	t        *testing.T
	f        *fakeFunctions
	srv      *httptest.Server
	project  string
	out, err bytes.Buffer
	ctx      context.Context
}

func newFnRig(t *testing.T) *fnRig {
	t.Helper()
	t.Setenv(tokenEnv, "plux_pat_fn")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	r := &fnRig{t: t, f: &fakeFunctions{}, project: t.TempDir(), ctx: context.Background()}
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewAppServiceHandler(fakeAppEnvs{}))
	mux.Handle(pluxv1connect.NewFunctionAdminServiceHandler(r.f))
	r.srv = httptest.NewServer(mux)
	t.Cleanup(r.srv.Close)
	return r
}

// run runs plux fn with the server and app flags added.
func (r *fnRig) run(args ...string) int {
	r.t.Helper()
	r.out.Reset()
	r.err.Reset()
	e := env{
		stdout: &r.out, stderr: &r.err, stdin: strings.NewReader(""), creds: credentials{dir: r.t.TempDir()}, httpClient: r.srv.Client(),
		pollInterval: time.Millisecond, ctx: r.ctx, vendor: func(context.Context, string) error { return nil },
	}
	full := append([]string{args[0]}, "--server", r.srv.URL, "--app", "a1", "-C", r.project)
	return e.fn(append(full, args[1:]...))
}

// crate writes a minimal function crate with its vendored dependencies.
func (r *fnRig) crate(name string) {
	r.t.Helper()
	dir := crateDir(r.project, name)
	for rel, content := range map[string]string{
		"Cargo.toml": "[package]\nname = \"bank_fee\"\n", "Cargo.lock": "version = 4\n", "src/lib.rs": "// code\n",
		"vendor/dep/Cargo.toml": "[package]\n", "target/plux/module.wasm": "not part of the source", ".cargo/config.toml": "[patch.crates-io]\n",
		"function.json": `{"schemaVersion":"1.0.0","kind":"function","id":"01a137b5-6b91-728a-b602-f8bbfb8dee11","name":"bank.fee","placement":"server"}`,
	} {
		putFile2(r.t, filepath.Join(dir, filepath.FromSlash(rel)), content)
	}
}

func putFile2(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	putFile(t, path, content)
}

func succeeded(tests ...*pluxv1.FunctionTestResult) *pluxv1.Build {
	return &pluxv1.Build{Id: "b-1", State: pluxv1.BuildState_BUILD_STATE_SUCCEEDED, TestResults: tests}
}

// untar lists the files of a source archive.
func untar(t *testing.T, archive []byte) map[string]string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(tr)
		out[h.Name] = string(data)
	}
}

// Verifies: FN-001, FN-008, FN-030.
// plux fn new writes a crate that depends on plux-fn = "0.1" through a
// .cargo patch, a typed handler for the placement, and a function.json that
// the function schema accepts with one stored test case.
func TestFnNew_FN_030(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	sdk := t.TempDir()
	putFile(t, filepath.Join(sdk, "Cargo.toml"), "[package]\nname = \"plux-fn\"\n")
	for _, placement := range []string{"server", "device"} {
		project := t.TempDir()
		var out, errOut bytes.Buffer
		e := env{stdout: &out, stderr: &errOut}
		if code := e.fn([]string{"new", "bank.transferFee", "--placement", placement, "--sdk", sdk, "-C", project}); code != exitOK {
			t.Fatalf("%s: %d %s", placement, code, errOut.String())
		}
		dir := crateDir(project, "bank.transferFee")
		read := func(rel string) string {
			data, err := os.ReadFile(filepath.Join(dir, rel))
			if err != nil {
				t.Fatal(err)
			}
			return string(data)
		}
		if c := read("Cargo.toml"); !strings.Contains(c, `name = "bank_transfer_fee"`) || !strings.Contains(c, `plux-fn = "0.1"`) || !strings.Contains(c, "schemars") || !strings.Contains(c, "serde_json") {
			t.Errorf("Cargo.toml:\n%s", c)
		}
		if c := read(".cargo/config.toml"); !strings.Contains(c, "[patch.crates-io]") || !strings.Contains(c, sdk) {
			t.Errorf("config.toml:\n%s", c)
		}
		want := map[string]string{"server": "ServerContext", "device": "DeviceContext"}[placement]
		if c := read("src/lib.rs"); !strings.Contains(c, "plux_fn::function!(handler);") || !strings.Contains(c, "_ctx: &"+want) || strings.Contains(c, "{{") {
			t.Errorf("lib.rs:\n%s", c)
		}
		files, err := collectSource(dir)
		if err != nil {
			t.Fatal(err)
		}
		decl, diags, err := readDeclaration(files, "bank.transferFee", limits.Defaults())
		if err != nil || decl == nil {
			t.Fatalf("function.json is not valid: %v %v", err, diags)
		}
		if string(decl.Function.Placement) != placement || len(decl.Function.Tests) != 1 || decl.Function.ID == "" {
			t.Errorf("declaration: %+v", decl.Function)
		}
	}
	e := env{stdout: io.Discard, stderr: io.Discard}
	if code := e.fn([]string{"new", "Bad_Name", "--sdk", sdk}); code != exitUsage {
		t.Errorf("a bad name: %d", code)
	}
	if code := e.fn([]string{"new", "bank.fee", "--placement", "cloud", "--sdk", sdk}); code != exitUsage {
		t.Errorf("a bad placement: %d", code)
	}
}

// Verifies: FN-006, FN-016, FN-031.
// plux fn deploy packs the crate, vendor directory, lock file and function
// document, uploads it in chunks, polls the build with backoff, prints the
// test results and deploys the version with the alias.
func TestFnDeploy_FN_006(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	r := newFnRig(t)
	r.crate("bank.fee")
	r.f.builds = []*pluxv1.Build{
		{Id: "b-1", State: pluxv1.BuildState_BUILD_STATE_QUEUED},
		{Id: "b-1", State: pluxv1.BuildState_BUILD_STATE_RUNNING},
		succeeded(&pluxv1.FunctionTestResult{Name: "adds", Passed: true}),
	}
	if code := r.run("deploy", "bank.fee", "--env", "development", "--alias", "stable"); code != exitOK {
		t.Fatalf("deploy: %d %s", code, r.err.String())
	}
	if r.out.String() != "deployed bank.fee version 4 (server, module abababababab), alias stable\n" {
		t.Errorf("stdout %q", r.out.String())
	}
	for _, want := range []string{"build b-1: queued", "build b-1: running", "build b-1: succeeded", "test adds: pass"} {
		if !strings.Contains(r.err.String(), want) {
			t.Errorf("stderr lacks %q:\n%s", want, r.err.String())
		}
	}
	up := r.f.upload
	if up.GetAppId() != "a1" || up.GetEnvironmentId() != "e-dev" || up.GetName() != "bank.fee" || len(up.GetChunk()) != 0 {
		t.Errorf("first message %v", up)
	}
	files := untar(t, r.f.uploaded)
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "Cargo.lock,Cargo.toml,function.json,src/lib.rs,vendor/dep/Cargo.toml" {
		t.Errorf("archive holds %v", names)
	}
	if r.f.deploy.GetBuildId() != "b-1" || r.f.deploy.GetAlias() != "stable" {
		t.Errorf("deploy %v", r.f.deploy)
	}
	if r.f.polls != 3 {
		t.Errorf("polled %d times", r.f.polls)
	}
	r.f.polls, r.f.builds = 0, []*pluxv1.Build{succeeded()}
	if code := r.run("deploy", "bank.fee", "--json"); code != exitOK || !strings.Contains(r.out.String(), `"version": 4`) {
		t.Errorf("--json: %d %s", code, r.out.String())
	}
}

// Verifies: FN-031.
// A build whose tests fail, or that fails, deploys nothing and exits 1 with
// the findings.
func TestFnDeployRefusals_FN_031(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	r := newFnRig(t)
	r.crate("bank.fee")
	r.f.builds = []*pluxv1.Build{succeeded(&pluxv1.FunctionTestResult{Name: "adds", Passed: true}, &pluxv1.FunctionTestResult{Name: "subtracts", Message: "expected 1; got 2"})}
	if code := r.run("deploy", "bank.fee"); code != exitFailed || r.f.deploy != nil {
		t.Errorf("failed tests: %d, deployed %v", code, r.f.deploy)
	}
	if !strings.Contains(r.err.String(), "test subtracts: FAIL (expected 1; got 2)") || !strings.Contains(r.err.String(), "nothing was deployed") {
		t.Errorf("stderr:\n%s", r.err.String())
	}

	r.f.polls = 0
	r.f.builds = []*pluxv1.Build{{Id: "b-1", State: pluxv1.BuildState_BUILD_STATE_FAILED, Diagnostics: []*pluxv1.Diagnostic{{
		Code: "PLX-7200", Severity: pluxv1.Severity_SEVERITY_ERROR, Message: "error[E0425]: cannot find value", Location: &pluxv1.Location{File: "src/lib.rs"},
	}}}}
	if code := r.run("deploy", "bank.fee"); code != exitFailed || r.f.deploy != nil {
		t.Errorf("failed build: %d, deployed %v", code, r.f.deploy)
	}
	if !strings.Contains(r.err.String(), "src/lib.rs: error PLX-7200: error[E0425]: cannot find value") {
		t.Errorf("stderr:\n%s", r.err.String())
	}

	r.f.builds = []*pluxv1.Build{succeeded()}
	r.f.upload, r.f.uploaded, r.f.polls = nil, nil, 0
	r.f.deployErr = connect.NewError(connect.CodePermissionDenied, errString("PLX-8030: function.deploy.production requires a second factor in this session"))
	if code := r.run("deploy", "bank.fee", "--env", "production"); code != exitFailed || !strings.Contains(r.err.String(), "step-up required") {
		t.Errorf("production: %d\n%s", code, r.err.String())
	}
	if r.f.upload.GetEnvironmentId() != "e-prod" {
		t.Errorf("environment %s", r.f.upload.GetEnvironmentId())
	}
	if code := r.run("deploy", "NotAName"); code != exitUsage {
		t.Errorf("a bad name: %d", code)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

// Verifies: FN-006.
func TestFnAliasAndVersions_FN_006(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	r := newFnRig(t)
	if code := r.run("alias", "bank.fee", "prod", "2"); code != exitOK || r.out.String() != "prod -> 2\n" {
		t.Fatalf("alias: %d %s %s", code, r.out.String(), r.err.String())
	}
	if a := r.f.alias; a.GetName() != "bank.fee" || a.GetAlias().GetName() != "prod" || a.GetAlias().GetVersion() != 2 || a.GetEnvironmentId() != "e-dev" {
		t.Errorf("request %v", a)
	}
	if code := r.run("alias", "bank.fee", "prod", "zero"); code != exitUsage {
		t.Errorf("a bad version: %d", code)
	}
	if code := r.run("alias", "bank.fee", "prod"); code != exitUsage {
		t.Errorf("missing argument: %d", code)
	}
	want := `VERSION  PLACEMENT  MODULE        TESTS   ALIASES      CREATED
2        server     cdcdcdcdcdcd  passed  prod,stable  2026-10-14T08:00:00Z
1        server     efefefefefef  passed               2026-10-14T08:00:00Z
`
	if code := r.run("versions", "bank.fee"); code != exitOK || r.out.String() != want {
		t.Errorf("versions: %d\n%s", code, r.out.String())
	}
	if code := r.run("versions", "bank.fee", "--json"); code != exitOK || !strings.Contains(r.out.String(), `"versions"`) {
		t.Errorf("versions --json: %d %s", code, r.out.String())
	}
}

// Verifies: FN-032.
// plux fn logs prints the lines oldest first, passes the version and the
// time filter, and with --follow prints only new lines until interrupted.
func TestFnLogs_FN_032(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	r := newFnRig(t)
	base := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	r.f.logs = []*pluxv1.FunctionLogLine{logLine(base.Add(2*time.Second), 2, "error", "boom"), logLine(base, 2, "info", "started")}
	if code := r.run("logs", "bank.fee", "--version", "2", "--since", "15m"); code != exitOK {
		t.Fatalf("logs: %d %s", code, r.err.String())
	}
	want := "2026-10-14T09:00:00Z v2 INFO  started\n2026-10-14T09:00:02Z v2 ERROR boom\n"
	if r.out.String() != want {
		t.Errorf("stdout:\n%s", r.out.String())
	}
	req := r.f.logReqs[0]
	if req.GetVersion() != 2 || req.GetName() != "bank.fee" || req.GetSince() == nil || time.Since(req.GetSince().AsTime()) < 14*time.Minute || time.Since(req.GetSince().AsTime()) > 16*time.Minute {
		t.Errorf("request %v", req)
	}
	if code := r.run("logs", "bank.fee", "--since", "yesterday"); code != exitUsage {
		t.Errorf("a bad --since: %d", code)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.ctx = ctx
	r.f.mu.Lock()
	r.f.logReqs = nil
	r.f.mu.Unlock()
	done := make(chan int)
	var outBuf bytes.Buffer
	go func() {
		e := env{stdout: &outBuf, stderr: io.Discard, creds: credentials{dir: t.TempDir()}, httpClient: r.srv.Client(), pollInterval: time.Millisecond, ctx: ctx}
		done <- e.fn([]string{"logs", "bank.fee", "--follow", "--server", r.srv.URL, "--app", "a1"})
	}()
	deadline := time.After(5 * time.Second)
	for {
		r.f.mu.Lock()
		n := len(r.f.logReqs)
		r.f.mu.Unlock()
		if n >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("follow did not poll")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	if code := <-done; code != exitOK {
		t.Errorf("follow: %d", code)
	}
	if outBuf.String() != want {
		t.Errorf("follow printed every line once, got:\n%s", outBuf.String())
	}
}

// Verifies: FN-019.
func TestPackArchiveIsDeterministic_FN_019(t *testing.T) {
	t.Parallel()
	r := &fnRig{t: t, project: t.TempDir()}
	r.crate("bank.fee")
	dir := crateDir(r.project, "bank.fee")
	files, err := collectSource(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := packArchive(files)
	time.Sleep(10 * time.Millisecond)
	b, _ := packArchive(files)
	if !bytes.Equal(a, b) {
		t.Error("the same source packed to different archives")
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "src", "link.rs")); err != nil {
		t.Skip("no symlinks")
	}
	if _, err := collectSource(dir); err == nil {
		t.Error("a symlink in src was accepted")
	}
}

// Verifies: FN-030.
// The same module answers the same cases on the compiling engine and on the
// interpreter that stands in for the device engine; a case that differs fails.
func TestInterpreterEngine_FN_030(t *testing.T) {
	t.Parallel()
	_ = json.Marshal
	outcomes := []caseOutcome{{Name: "a", Wasm: true}, {Name: "b", Wasm: true, Interpreter: boolPtr(false), Message: "interpreter: differs"}}
	table := caseTable(outcomes, true)
	if !strings.Contains(table, "CASE  WASM  INTERPRETER") || !strings.Contains(table, "b     pass  FAIL") || !strings.Contains(table, "b: interpreter: differs") {
		t.Errorf("table:\n%s", table)
	}
}

func boolPtr(b bool) *bool { return &b }

// Verifies: FN-030.
// plux fn new, build, test and invoke with the real toolchain: a scaffolded
// function builds, its stored case passes natively, on the compiling engine
// and, for a device function, on the interpreter, and it runs locally.
func TestFnEndToEnd_FN_030(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	if os.Getenv("PLUX_RUST_E2E") != "1" {
		t.Skip("set PLUX_RUST_E2E=1 to build with the real Rust toolchain and Binaryen")
	}
	sdk, err := filepath.Abs("../../../sdk/fn/plux-fn")
	if err != nil {
		t.Fatal(err)
	}
	for _, placement := range []string{"server", "device"} {
		t.Run(placement, func(t *testing.T) {
			project := t.TempDir()
			started := time.Now()
			run := func(args ...string) (int, string, string) {
				var out, errOut bytes.Buffer
				e := env{stdout: &out, stderr: &errOut, stdin: strings.NewReader("")}
				return e.fn(append(args, "-C", project)), out.String(), errOut.String()
			}
			const name = "demo.greet"
			if code, _, errOut := run("new", name, "--placement", placement, "--sdk", sdk); code != exitOK {
				t.Fatalf("new: %d %s", code, errOut)
			}
			if code, out, errOut := run("test", name, "--sdk", sdk); code != exitOK {
				t.Fatalf("test: %d\n%s\n%s", code, out, errOut)
			} else if !strings.Contains(out, "greets by name") || (placement == "device") != strings.Contains(out, "INTERPRETER") {
				t.Errorf("table:\n%s", out)
			}
			code, out, errOut := run("invoke", name, "--input", `{"name":"Grace"}`)
			if code != exitOK || strings.TrimSpace(out) != `{"greeting":"Hello, Grace!"}` {
				t.Errorf("invoke: %d %q %s", code, out, errOut)
			}
			if code, _, errOut := run("invoke", name, "--input", `{"name":""}`); code != exitFailed || !strings.Contains(errOut, "NAME_EMPTY") {
				t.Errorf("invoke business error: %d %s", code, errOut)
			}
			for _, f := range []string{moduleName, "input.schema.json", "output.schema.json", "provenance.json"} {
				if _, err := os.Stat(filepath.Join(crateDir(project, name), outputDir, f)); err != nil {
					t.Error(err)
				}
			}
			t.Logf("%s: new, test and invoke took %s", placement, time.Since(started).Round(time.Second))
		})
	}
}
