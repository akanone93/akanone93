// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//go:build quickstart

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// dryRunBudget is how long the steps of the quick start that need no device
// may take together; the guide gives the whole path ten minutes.
const dryRunBudget = 5 * time.Minute

// TestQuickstartDryRun times the scripted steps of the quick start that run
// without a device: the starter project from `plux create --router
// go_router`, its packages, its analysis, and the validation and local build
// of the starter Plux project. It writes the timings as JSON to the file
// named by PLUX_QUICKSTART_REPORT and fails when they exceed the budget. Run
// by `make quickstart-dry-run`; the recorded sessions with people decide the
// status of the requirements.
// Verifies: HST-034, DX-001.
func TestQuickstartDryRun(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	repo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	flutter, err := exec.LookPath("flutter")
	if err != nil {
		t.Fatalf("the dry run needs Flutter on the PATH: %v", err)
	}
	t.Setenv(tokenEnv, "plux_pat_test")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	f, url := newFake(t)
	serveRelease(t, f)

	type step struct {
		Name    string  `json:"name"`
		Seconds float64 `json:"seconds"`
	}
	var steps []step
	total := time.Now()
	timed := func(name string, run func()) {
		t.Helper()
		start := time.Now()
		run()
		steps = append(steps, step{Name: name, Seconds: time.Since(start).Seconds()})
	}
	project := filepath.Join(t.TempDir(), "quickstart")
	flutterIn := func(dir string, args ...string) func() {
		return func() {
			t.Helper()
			cmd := exec.CommandContext(t.Context(), flutter, args...) //nolint:gosec // G204: the dry run's own tool and arguments
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("flutter %v: %v\n%s", args, err, out)
			}
		}
	}

	timed("plux create --router go_router", func() {
		args := []string{
			"create", "--server", url, "--application-id", "com.example.quickstart", "--bundle-id", "com.example.quickstart",
			"--router", "go_router", "--plux-path", filepath.Join(repo, "packages", "plux_flutter"), "--out", project, "demo",
		}
		if code, stdout, stderr := cli(t, t.TempDir(), args...); code != exitOK {
			t.Fatalf("create: %d %s %s", code, stdout, stderr)
		}
	})
	timed("flutter pub get", flutterIn(project, "pub", "get"))
	timed("flutter analyze", flutterIn(project, "analyze"))

	starter := filepath.Join(repo, "schema", "testdata", "documents", "starter")
	timed("plux validate", func() {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"validate", starter}, &stdout, &stderr); code != exitOK {
			t.Fatalf("validate: %d %s %s", code, &stdout, &stderr)
		}
	})
	out := filepath.Join(t.TempDir(), "out")
	timed("plux build", func() {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"build", "-o", out, starter}, &stdout, &stderr); code != exitOK {
			t.Fatalf("build: %d %s %s", code, &stdout, &stderr)
		}
	})

	elapsed := time.Since(total)
	report, err := json.MarshalIndent(map[string]any{
		"requirements":  []string{"HST-034", "DX-001"},
		"steps":         steps,
		"totalSeconds":  elapsed.Seconds(),
		"budgetSeconds": dryRunBudget.Seconds(),
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s", report)
	if path := os.Getenv("PLUX_QUICKSTART_REPORT"); path != "" {
		if err := os.WriteFile(path, append(report, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed > dryRunBudget {
		t.Errorf("the scripted steps took %s, over the budget of %s", elapsed, dryRunBudget)
	}
}
