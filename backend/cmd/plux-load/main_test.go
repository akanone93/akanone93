// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/loadgen"
	"github.com/nightCode42/plux3/backend/internal/loadgen/loadgentest"
)

// Verifies: DEP-051.
// A small run prints the JSON report on stdout and exits 0.
func TestRunPrintsTheReport_DEP_051(t *testing.T) {
	t.Parallel()
	srv := loadgentest.New(t)
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), []string{
		"-server", srv.URL, "-app", "app", "-devices", "3", "-duration", "800ms", "-speedup", "600", "-function", "demo.echo",
	}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	var report loadgen.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("%v in %s", err, stdout.String())
	}
	if report.Schema != loadgen.ReportSchema || report.Registered != 3 || report.PacingScale != 600 || report.Errors() != 0 {
		t.Errorf("report = %+v", report)
	}
	if report.Operations[loadgen.OpControl].Requests < 3 || report.Operations[loadgen.OpFunction].Requests == 0 {
		t.Errorf("operations = %+v", report.Operations)
	}
}

// Verifies: DEP-051.
// -device-ids-file lists the registered devices, one ID per line, so that an
// audit script can name a device of the run.
func TestRunWritesDeviceIDs_DEP_051(t *testing.T) {
	t.Parallel()
	srv := loadgentest.New(t)
	file := filepath.Join(t.TempDir(), "ids.txt")
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), []string{"-server", srv.URL, "-app", "app", "-devices", "3", "-duration", "300ms", "-speedup", "600", "-device-ids-file", file}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Fields(string(data)); len(lines) != 3 {
		t.Errorf("device IDs = %q, want three", data)
	}
	if code := run(t.Context(), []string{"-server", srv.URL, "-app", "app", "-devices", "1", "-duration", "100ms", "-device-ids-file", t.TempDir()}, &stdout, &stderr); code != exitFailed {
		t.Errorf("an unwritable file: exit %d, want %d", code, exitFailed)
	}
}

// Verifies: DEP-051.
// A usage error exits 2, a server that is not there exits 1, and a run whose
// errors exceed the limit exits 1 after printing its report.
func TestRunExitCodes_DEP_051(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	for name, args := range map[string][]string{
		"no server":      {"-app", "a"},
		"bad speedup":    {"-server", "http://x", "-app", "a", "-speedup", "0"},
		"bad function":   {"-server", "http://x", "-app", "a", "-function", "f", "-function-input", "{"},
		"unknown flag":   {"-nonsense"},
		"missing CA":     {"-server", "http://x", "-app", "a", "-ca-file", "/nonexistent.pem"},
		"zero devices":   {"-server", "http://x", "-app", "a", "-devices", "0"},
		"negative range": {"-server", "http://x", "-app", "a", "-duration", "-1s"},
	} {
		if code := run(t.Context(), args, &out, &errOut); code != exitUsage {
			t.Errorf("%s: exit %d, want %d", name, code, exitUsage)
		}
	}
	if code := run(t.Context(), []string{"-server", "http://127.0.0.1:1", "-app", "a", "-devices", "1", "-duration", "1s"}, &out, &errOut); code != exitFailed {
		t.Errorf("no server: exit %d, want %d", code, exitFailed)
	}
	if code := run(t.Context(), []string{"-h"}, &out, &errOut); code != exitOK {
		t.Errorf("-h: exit %d", code)
	}
}

// Verifies: DEP-051.
// The verdict fails a run over the error limit and one that made no request.
func TestVerdict_DEP_051(t *testing.T) {
	t.Parallel()
	ok := loadgen.Report{Operations: map[string]loadgen.OpReport{"manifest": {Requests: 1000, Errors: 1}}}
	bad := loadgen.Report{Operations: map[string]loadgen.OpReport{"manifest": {Requests: 1000, Errors: 5}}}
	var errOut bytes.Buffer
	if verdict(ok, 0.001, &errOut) != exitOK || verdict(bad, 0.001, &errOut) != exitFailed || verdict(loadgen.Report{}, 1, &errOut) != exitFailed {
		t.Errorf("verdicts wrong: %s", errOut.String())
	}
}
