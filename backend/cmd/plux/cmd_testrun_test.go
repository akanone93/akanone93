// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const starterProject = "../../../schema/testdata/documents/starter"

// copyProject copies the starter project into a temporary directory,
// without its scenario files.
func copyProject(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	root := os.DirFS(starterProject)
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(p, "tests/") {
			return err
		}
		data, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
			return err
		}
		return os.WriteFile(to, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func putScenario(t *testing.T, dir, path, text string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}
	putFile(t, full, text)
}

func runTest(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(append([]string{"test"}, args...), &out, &errb)
	return code, out.String(), errb.String()
}

const profileScenario = `schemaVersion: 1.0.0
kind: scenarios
scenarios:
  - name: opens the profile
    page: place
    given:
      params:
        name: Harbour
    steps:
      - tap: place-profile
    expect:
      - navigatedTo: profile
`

// fakeFlutterReporting is a flutter executable that reports one scenario
// with the given result line.
func fakeFlutterReporting(t *testing.T, doneLines string, exit string) string {
	t.Helper()
	dir := t.TempDir()
	report := `{"test":{"id":2,"name":"tests/a.scenario.yaml opens the profile","suiteID":0,"groupIDs":[1]},"type":"testStart","time":1000}` + "\n" + doneLines +
		`{"success":true,"type":"done","time":2000}` + "\n"
	putFile(t, filepath.Join(dir, "report.txt"), report)
	path := filepath.Join(dir, "flutter")
	if err := os.WriteFile(path, []byte("#!/bin/sh\ncat \""+filepath.Join(dir, "report.txt")+"\"\nexit "+exit+"\n"), 0o700); err != nil { //nolint:gosec // an executable test double
		t.Fatal(err)
	}
	return path
}

// Verifies: TST-002.
func TestTestCommandRunsScenariosReportsAndWritesJUnit(t *testing.T) {
	t.Parallel()
	dir := copyProject(t)
	putScenario(t, dir, "tests/a.scenario.yaml", profileScenario)
	junit := filepath.Join(t.TempDir(), "out", "junit.xml")
	pass := fakeFlutterReporting(t, `{"testID":2,"result":"success","skipped":false,"hidden":false,"type":"testDone","time":1500}`+"\n", "0")
	code, stdout, stderr := runTest(t, "-C", dir, "--flutter", pass, "--junit", junit)
	if code != exitOK || !strings.Contains(stdout, "PASS  tests/a.scenario.yaml: opens the profile") || !strings.Contains(stdout, "1 scenarios, 1 passed") {
		t.Fatalf("passing run: %d\n%s\n%s", code, stdout, stderr)
	}
	if xml := read(t, junit); !strings.Contains(xml, `<testcase classname="tests/a.scenario.yaml" name="opens the profile"`) || !strings.Contains(xml, `failures="0"`) {
		t.Errorf("junit = %s", xml)
	}

	fail := fakeFlutterReporting(t, `{"testID":2,"messageType":"print","message":"The following TestFailure was thrown running a test:\ntests/a.scenario.yaml:12:9: expected a navigation to \"x\"\n\nWhen the exception was thrown","type":"print","time":1100}`+"\n"+
		`{"testID":2,"error":"Test failed. See exception logs above.","stackTrace":"","isFailure":false,"type":"error","time":1200}`+"\n"+
		`{"testID":2,"result":"error","skipped":false,"hidden":false,"type":"testDone","time":1300}`+"\n", "1")
	code, stdout, _ = runTest(t, "-C", dir, "--flutter", fail, "--junit", junit)
	if code != exitFailed || !strings.Contains(stdout, "FAIL  tests/a.scenario.yaml: opens the profile") || !strings.Contains(stdout, "a.scenario.yaml:12:9: expected a navigation") {
		t.Fatalf("failing run: %d\n%s", code, stdout)
	}
	if xml := read(t, junit); !strings.Contains(xml, `<failure message="tests/a.scenario.yaml:12:9`) {
		t.Errorf("junit = %s", xml)
	}
}

// Verifies: TST-002.
func TestTestCommandKeepsTheGeneratedProject(t *testing.T) {
	t.Parallel()
	dir := copyProject(t)
	putScenario(t, dir, "tests/a.scenario.yaml", profileScenario)
	work := filepath.Join(t.TempDir(), "work")
	pass := fakeFlutterReporting(t, `{"testID":2,"result":"success","skipped":false,"hidden":false,"type":"testDone","time":1500}`+"\n", "0")
	if code, _, stderr := runTest(t, "-C", dir, "--flutter", pass, "--work", work, "--runtime", "/src/plux_flutter"); code != exitOK {
		t.Fatalf("%d %s", code, stderr)
	}
	if pub := read(t, filepath.Join(work, "pubspec.yaml")); !strings.Contains(pub, "path: '/src/plux_flutter'") {
		t.Errorf("pubspec = %s", pub)
	}
	if _, err := os.Stat(filepath.Join(work, "test", "01_tests_a_scenario_yaml_test.dart")); err != nil {
		t.Error(err)
	}
}

// Verifies: TST-001.
func TestTestCommandReportsScenarioErrorsAtTheirLine(t *testing.T) {
	t.Parallel()
	dir := copyProject(t)
	putScenario(t, dir, "tests/bad.scenario.yaml", "schemaVersion: 1.0.0\nkind: scenarios\nscenarios:\n  - name: n\n    page: place\n    expect:\n      - bogus: 1\n")
	code, _, stderr := runTest(t, "-C", dir, "--flutter", "/does/not/matter")
	if code != exitFailed || !strings.Contains(stderr, "tests/bad.scenario.yaml:7:9: error PLX-1270") {
		t.Errorf("%d\n%s", code, stderr)
	}
	putScenario(t, dir, "tests/bad.scenario.yaml", "schemaVersion: 1.0.0\nkind: scenarios\nscenarios:\n  - name: n\n    page: nowhere\n    expect:\n      - visible: a\n")
	code, _, stderr = runTest(t, "-C", dir, "--flutter", "/does/not/matter")
	if code != exitFailed || !strings.Contains(stderr, "tests/bad.scenario.yaml:5:5: error PLX-1272") {
		t.Errorf("%d\n%s", code, stderr)
	}
}

// Verifies: TST-001.
func TestTestCommandFindsScenariosThroughPluxYAML(t *testing.T) {
	t.Parallel()
	dir := copyProject(t)
	putScenario(t, dir, "specs/a.scenario.yaml", profileScenario)
	putFile(t, filepath.Join(dir, "plux.yaml"), "tests: specs/*.scenario.yaml\n")
	pass := fakeFlutterReporting(t, `{"testID":2,"result":"success","skipped":false,"hidden":false,"type":"testDone","time":1500}`+"\n", "0")
	// The fake names its scenario after tests/a.scenario.yaml, so the run
	// cannot match it: what matters is that specs/ was found and generated.
	work := filepath.Join(t.TempDir(), "work")
	_, stdout, _ := runTest(t, "-C", dir, "--flutter", pass, "--work", work)
	if !strings.Contains(stdout, "specs/a.scenario.yaml: opens the profile") {
		t.Errorf("stdout = %s", stdout)
	}
}

// Verifies: TST-002.
func TestTestCommandUsageAndMissingPieces(t *testing.T) {
	t.Parallel()
	if code, _, stderr := runTest(t, "-h"); code != exitOK || !strings.Contains(stderr, "Usage: plux test") {
		t.Errorf("-h: %d %s", code, stderr)
	}
	if code, _, _ := runTest(t, "extra"); code != exitUsage {
		t.Errorf("a positional argument: %d", code)
	}
	if code, _, _ := runTest(t, "--nope"); code != exitUsage {
		t.Errorf("an unknown flag: %d", code)
	}
	if code, _, _ := runTest(t, "-C", filepath.Join(t.TempDir(), "missing")); code != exitUsage {
		t.Errorf("a missing directory: %d", code)
	}
	empty := copyProject(t)
	code, stdout, stderr := runTest(t, "-C", empty)
	if code != exitOK || !strings.Contains(stdout, "no scenarios to run") || !strings.Contains(stderr, "PLX-1276") {
		t.Errorf("no scenarios: %d %s %s", code, stdout, stderr)
	}
	dir := copyProject(t)
	putScenario(t, dir, "tests/a.scenario.yaml", profileScenario)
	if code, _, stderr := runTest(t, "-C", dir, "--flutter", filepath.Join(t.TempDir(), "missing")); code != exitFailed || !strings.Contains(stderr, "PLX-1275") {
		t.Errorf("missing flutter: %d %s", code, stderr)
	}
}

// Verifies: TST-001, TST-002.
// plux test on the starter project, with the Flutter SDK, end to end.
func TestTestCommandRunsTheStarterProject(t *testing.T) {
	t.Parallel()
	flutter, err := exec.LookPath("flutter")
	if err != nil {
		t.Skip("flutter is not installed: ", err)
	}
	runtime, err := filepath.Abs("../../../packages/plux_flutter")
	if err != nil {
		t.Fatal(err)
	}
	junit := filepath.Join(t.TempDir(), "junit.xml")
	code, stdout, stderr := runTest(t, "-C", starterProject, "--flutter", flutter, "--runtime", runtime, "--junit", junit)
	if code != exitOK || !strings.Contains(stdout, "PASS  tests/place.scenario.yaml") {
		t.Fatalf("%d\n%s\n%s", code, stdout, stderr)
	}
	if xml := read(t, junit); !strings.Contains(xml, `tests="1" failures="0" errors="0"`) {
		t.Errorf("junit = %s", xml)
	}
}

// fakeTestRuns records the runs `plux test --report` reports.
type fakeTestRuns struct {
	pluxv1connect.UnimplementedTestRunServiceHandler
	got *[]*pluxv1.ReportRunRequest
}

func (f fakeTestRuns) ReportRun(_ context.Context, r *connect.Request[pluxv1.ReportRunRequest]) (*connect.Response[pluxv1.ReportRunResponse], error) {
	if r.Header().Get("Authorization") != "Bearer plux_pat_run" || r.Header().Get("X-Plux-Organization") != "org-1" {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	*f.got = append(*f.got, r.Msg)
	return connect.NewResponse(&pluxv1.ReportRunResponse{Run: &pluxv1.TestRun{Id: "run-1"}}), nil
}

// Verifies: TST-002.
// plux test --report records the run on the server with the caller's
// credential: one scenario result each, the failure message as a step, and
// the exit code of the tests unless only the report fails.
func TestTestCommandReportsTheRun_TST_002(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_run")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	var got []*pluxv1.ReportRunRequest
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewTestRunServiceHandler(fakeTestRuns{got: &got}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	dir := copyProject(t)
	putScenario(t, dir, "tests/a.scenario.yaml", profileScenario)
	pass := fakeFlutterReporting(t, `{"testID":2,"result":"success","skipped":false,"hidden":false,"type":"testDone","time":1500}`+"\n", "0")
	fail := fakeFlutterReporting(t, `{"testID":2,"messageType":"print","message":"The following TestFailure was thrown running a test:\ntests/a.scenario.yaml:12:9: expected a navigation to \"x\"\n\nWhen the exception was thrown","type":"print","time":1100}`+"\n"+
		`{"testID":2,"error":"Test failed. See exception logs above.","stackTrace":"","isFailure":false,"type":"error","time":1200}`+"\n"+
		`{"testID":2,"result":"error","skipped":false,"hidden":false,"type":"testDone","time":1300}`+"\n", "1")
	test := func(flutter string, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		e := newEnv(&out, &errOut)
		e.creds = credentials{dir: t.TempDir()}
		full := append([]string{"-C", dir, "--flutter", flutter, "--report", "--server", srv.URL, "--org", "org-1"}, args...)
		return e.testCmd(full), out.String(), errOut.String()
	}

	code, out, stderr := test(pass, "--app", "app-1", "--label", "main", "--report-key", "k1")
	if code != exitOK || !strings.Contains(out, "reported run run-1") || len(got) != 1 {
		t.Fatalf("passing run: %d %q %s %d", code, out, stderr, len(got))
	}
	if r := got[0]; r.GetAppId() != "app-1" || r.GetReportKey() != "k1" || r.GetSource() != "cli" || r.GetLabel() != "main" || len(r.GetScenarios()) != 1 ||
		r.GetScenarios()[0].GetStatus() != "passed" || r.GetScenarios()[0].GetName() != "opens the profile" || len(r.GetScenarios()[0].GetSteps()) != 0 {
		t.Errorf("passing report = %v", r)
	}

	code, _, _ = test(fail, "--app", "app-1")
	if code != exitFailed || len(got) != 2 || len(got[1].GetReportKey()) != 32 {
		t.Fatalf("failing run: %d %d", code, len(got))
	}
	sc := got[1].GetScenarios()[0]
	if sc.GetStatus() != "failed" || len(sc.GetSteps()) != 1 || !strings.Contains(sc.GetSteps()[0].GetMessage(), "a.scenario.yaml:12:9") || sc.GetSteps()[0].GetStatus() != "failed" {
		t.Errorf("failing report = %v", sc)
	}

	if code, _, stderr = test(pass); code != exitUsage || !strings.Contains(stderr, "no app") {
		t.Errorf("no app: %d %s", code, stderr)
	}
	t.Setenv(tokenEnv, "")
	if code, _, stderr = test(pass, "--app", "app-1"); code != exitAuth || !strings.Contains(stderr, "not signed in") || len(got) != 2 {
		t.Errorf("not signed in: %d %s", code, stderr)
	}
	t.Setenv(tokenEnv, "plux_pat_other")
	if code, _, stderr = test(pass, "--app", "app-1"); code != exitAuth || len(got) != 2 {
		t.Errorf("refused: %d %s", code, stderr)
	}
}
