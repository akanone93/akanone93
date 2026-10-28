// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/plxtest"
)

// testCmd runs a project's test scenarios headlessly (TST-002): it
// compiles the project, generates a Flutter test project that starts the
// real runtime on a release signed with a key made for the run, runs
// `flutter test` and reports the result. With --report the run is also
// recorded on the server, where Studio lists it (TST-002). The logic is in
// plxtest.
func (e env) testCmd(args []string) int {
	stdout, stderr := e.stdout, e.stderr
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	dir := set.String("C", ".", "the project `directory`")
	junit := set.String("junit", "", "write the results as JUnit XML to `file`")
	flutter := set.String("flutter", "", "the `path` of the Flutter executable (default flutter on the PATH)")
	runtime := set.String("runtime", "", "use the plux_flutter checkout in `dir` instead of the published package")
	work := set.String("work", "", "generate the Flutter test project in `dir` and keep it (default a temporary directory)")
	var c common
	c.register(set, false)
	set.StringVar(&c.app, "app", "", "the app `id` to report to (default plux.json)")
	var rf reportFlags
	rf.register(set)
	set.Usage = func() {
		_, _ = fmt.Fprint(set.Output(), "Usage: plux test [-C dir] [--junit file] [--flutter path]\n\n"+
			"Runs the test scenarios of a project (tests/**/*.scenario.yaml or .json, or the glob\n"+
			"named by tests in plux.yaml) headlessly with Flutter's test harness and the real runtime:\n"+
			"the project is compiled and signed with a key made for the run, and each scenario\n"+
			"becomes one test. With --report the run is recorded on the server with the caller's\n"+
			"credential, like the other server commands. Exit codes: 0 every scenario passed,\n"+
			"1 a scenario failed or the project or a scenario file has errors, 2 usage error,\n"+
			"3 not signed in or refused, 4 server unreachable.\n\nFlags:\n")
		set.PrintDefaults()
	}
	set.SetOutput(stderr)
	if err := set.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if set.NArg() != 0 {
		set.Usage()
		return exitUsage
	}
	fsys, ok := project("test", *dir, stderr)
	if !ok {
		return exitUsage
	}
	c.dir = *dir
	rep, err := e.reporterFor(c, rf)
	if err != nil {
		return e.fail("test", err)
	}
	cfg, err := readHostConfig(*dir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s test: %v\n", name, err)
		return exitFailed
	}
	opts := plxtest.Options{
		Project: fsys, Compiler: options(false), Tests: cfg.Tests, Entropy: rand.Reader,
		Runtime: plxtest.Dependency{Version: runtimeConstraint, Path: *runtime},
	}
	plan, err := plxtest.Prepare(opts)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s test: %v\n", name, err)
		return exitFailed
	}
	for _, d := range plan.Compile {
		_, _ = fmt.Fprintln(stderr, d.String())
	}
	for _, f := range plan.Findings {
		_, _ = fmt.Fprintln(stderr, f.String())
	}
	if !plan.Runnable() {
		if len(plan.Cases) == 0 && !plan.Compile.HasErrors() && !plxtest.HasErrors(plan.Findings) {
			_, _ = fmt.Fprintf(stdout, "no scenarios to run\n")
			return exitOK
		}
		return exitFailed
	}
	return e.runPlan(plan, *work, *flutter, *junit, rep)
}

// reportFlags are the flags of `plux test` that concern the report.
type reportFlags struct {
	on                 bool
	label, source, key string
}

func (f *reportFlags) register(set *flag.FlagSet) {
	set.BoolVar(&f.on, "report", false, "record the run on the Plux Server, where Studio lists it (needs plugin.edit)")
	set.StringVar(&f.label, "label", "", "name what was tested, e.g. a branch or a commit, in the reported run")
	set.StringVar(&f.source, "source", "cli", "where the reported run came from, e.g. cli or ci")
	set.StringVar(&f.key, "report-key", "", "the `key` that makes the report idempotent (default random)")
}

// reporterFor connects to the server for --report; without it, nil.
func (e env) reporterFor(c common, f reportFlags) (*reporter, error) {
	if !f.on {
		return nil, nil //nolint:nilnil // no report was asked for
	}
	if err := c.resolve(); err != nil {
		return nil, err
	}
	if c.app == "" {
		return nil, usageError("no app: pass --app or run plux init")
	}
	client, err := e.testRunClient(c)
	if err != nil {
		return nil, err
	}
	return &reporter{client: client, app: c.app, source: f.source, label: f.label, key: f.key}, nil
}

// reporter records a run on the server.
type reporter struct {
	client                  pluxv1connect.TestRunServiceClient
	app, source, label, key string
}

// testRunClient connects to the test-run service with the caller's token.
func (e env) testRunClient(c common) (pluxv1connect.TestRunServiceClient, error) {
	opts, err := e.authOptions(c)
	if err != nil {
		return nil, err
	}
	return pluxv1connect.NewTestRunServiceClient(e.httpClient, c.server, opts), nil
}

// scenarioResults converts the results of a run to the wire form: one
// scenario each, with one step carrying the failure message of a scenario
// that did not pass (the harness reports scenarios, not their steps).
func scenarioResults(results []plxtest.CaseResult) []*pluxv1.TestScenarioResult {
	out := make([]*pluxv1.TestScenarioResult, 0, len(results))
	for i, r := range results {
		id := fmt.Sprintf("%s#%d", r.Case.File, i)
		if r.Case.Line > 0 {
			id = fmt.Sprintf("%s:%d", r.Case.File, r.Case.Line)
		}
		status := "failed"
		switch r.Status {
		case plxtest.Passed:
			status = "passed"
		case plxtest.Skipped:
			status = "skipped"
		case plxtest.Failed, plxtest.Errored, plxtest.NotRun:
		}
		sc := &pluxv1.TestScenarioResult{Id: id, Name: r.Case.Name, Status: status, DurationMs: r.Millis}
		if status == "failed" {
			message := r.Message
			if message == "" {
				message = "the scenario did not run to the end"
			}
			sc.Steps = []*pluxv1.TestStepResult{{Index: 0, Description: r.Case.Name, Status: "failed", Message: message}}
		}
		out = append(out, sc)
	}
	return out
}

// report records the results and says so; a retry of the same key returns
// the first run.
func (e env) report(r *reporter, results []plxtest.CaseResult) error {
	key := r.key
	if key == "" {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return fmt.Errorf("make a report key: %w", err)
		}
		key = hex.EncodeToString(b[:])
	}
	res, err := r.client.ReportRun(context.Background(), connect.NewRequest(&pluxv1.ReportRunRequest{
		AppId: r.app, ReportKey: key, Source: r.source, Label: r.label, Scenarios: scenarioResults(results),
	}))
	if err != nil {
		return fmt.Errorf("report the run: %w", err)
	}
	_, _ = fmt.Fprintf(e.stdout, "reported run %s\n", res.Msg.GetRun().GetId())
	return nil
}

// runPlan writes the Flutter project, runs it and reports.
func (e env) runPlan(plan *plxtest.Plan, work, flutter, junit string, rep *reporter) int {
	stdout, stderr := e.stdout, e.stderr
	harnessFailed := func(format string, args ...any) int {
		d := plxerr.NewDiagnostic(plxerr.TestHarnessFailed, plxerr.Location{}, format, args...)
		_, _ = fmt.Fprintln(stderr, d.String())
		return exitFailed
	}
	if flutter == "" {
		path, err := exec.LookPath("flutter")
		if err != nil {
			return harnessFailed("flutter is not on the PATH; install it or pass --flutter")
		}
		flutter = path
	}
	if work == "" {
		tmp, err := os.MkdirTemp("", "plux-test-")
		if err != nil {
			return harnessFailed("%v", err)
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		work = tmp
	}
	if err := plan.Write(work); err != nil {
		return harnessFailed("%v", err)
	}
	report, err := plxtest.Flutter(context.Background(), flutter, work, stderr)
	if err != nil {
		return harnessFailed("%v", err)
	}
	results, err := plxtest.ParseReport(bytes.NewReader(report), plan.Cases)
	if err != nil {
		return harnessFailed("%v", err)
	}
	printResults(stdout, results)
	sum := plxtest.Summarise(results)
	_, _ = fmt.Fprintf(stdout, "%d scenarios, %d passed, %d failed, %d errors, %d skipped\n", sum.Total, sum.Passed, sum.Failed, sum.Errored, sum.Skipped)
	if junit != "" {
		xmlData, err := plxtest.JUnit(results)
		if err == nil {
			err = writeJUnit(junit, xmlData)
		}
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s test: %v\n", name, err)
			return exitFailed
		}
	}
	reported := exitOK
	if rep != nil {
		if err := e.report(rep, results); err != nil {
			reported = e.fail("test", err)
		}
	}
	if !sum.OK() {
		return exitFailed
	}
	return reported
}

// printResults prints one line per scenario, with the message of a failure.
func printResults(stdout io.Writer, results []plxtest.CaseResult) {
	for _, r := range results {
		mark := "PASS"
		switch r.Status {
		case plxtest.Failed:
			mark = "FAIL"
		case plxtest.Errored, plxtest.NotRun:
			mark = "ERROR"
		case plxtest.Skipped:
			mark = "SKIP"
		case plxtest.Passed:
		}
		_, _ = fmt.Fprintf(stdout, "%-5s %s: %s\n", mark, r.Case.File, r.Case.Name)
		if r.Message != "" {
			_, _ = fmt.Fprintf(stdout, "      %s\n", r.Message)
		}
	}
}

// writeJUnit writes the JUnit file, creating its directory.
func writeJUnit(path string, data []byte) error {
	dir, err := openDirectory(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return writeFile(path, data)
}
