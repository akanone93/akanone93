// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"strings"
	"testing"
	"time"
)

// shell returns a command that runs script with sh.
func shell(script string, lim Limits) Command {
	return Command{Path: "/bin/sh", Args: []string{"-c", script}, Env: []string{"PATH=/usr/bin:/bin"}, Limits: lim}
}

// Verifies: FN-016.
func TestNoSandboxCapturesOutputAndExitCode_FN_016(t *testing.T) {
	res, err := NoSandbox{}.Run(t.Context(), shell("echo out; echo err >&2; exit 3", Limits{OutputBytes: 1024}))
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 3 || string(res.Stdout) != "out\n" || string(res.Stderr) != "err\n" {
		t.Errorf("result = %+v", res)
	}
}

// Verifies: FN-016.
func TestNoSandboxGivesTheChildOnlyItsEnvironment_FN_016(t *testing.T) {
	t.Setenv("PLUX_LEAK", "secret")
	cmd := shell("echo \"[$PLUX_LEAK][$ONLY]\"", Limits{})
	cmd.Env = append(cmd.Env, "ONLY=yes")
	res, err := NoSandbox{}.Run(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(res.Stdout)); got != "[][yes]" {
		t.Errorf("environment leaked: %q", got)
	}
}

// Verifies: FN-016.
func TestNoSandboxKillsTheGroupAtTheWallTime_FN_016(t *testing.T) {
	start := time.Now()
	res, err := NoSandbox{}.Run(t.Context(), shell("sleep 30 & sleep 30", Limits{Wall: 300 * time.Millisecond}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.TimedOut {
		t.Errorf("TimedOut = false, result %+v", res)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("took %s; the process group was not killed", elapsed)
	}
}

// Verifies: FN-016.
func TestNoSandboxKillsAChildThatWritesTooMuch_FN_016(t *testing.T) {
	res, err := NoSandbox{}.Run(t.Context(), shell("yes", Limits{OutputBytes: 4096, Wall: 20 * time.Second}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.OutputExceeded || res.TimedOut || len(res.Stdout) != 4096 {
		t.Errorf("OutputExceeded=%v TimedOut=%v len(stdout)=%d", res.OutputExceeded, res.TimedOut, len(res.Stdout))
	}
}

func TestNoSandboxReturnsTheContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := (NoSandbox{}).Run(ctx, shell("sleep 5", Limits{})); err == nil {
		t.Error("expected an error for a cancelled context")
	}
}

func TestNoSandboxRejectsBadCommands(t *testing.T) {
	if _, err := (NoSandbox{}).Run(t.Context(), Command{}); err == nil {
		t.Error("expected an error for an empty path")
	}
	if _, err := (NoSandbox{}).Run(t.Context(), Command{Path: "/nonexistent/tool"}); err == nil {
		t.Error("expected an error for a missing program")
	}
}

func TestBoundedBufferKeepsTheFirstBytes(t *testing.T) {
	fired := 0
	b := newBoundedBuffer(5, func() { fired++ })
	for _, s := range []string{"abc", "defg", "hij"} {
		if n, err := b.Write([]byte(s)); n != len(s) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", s, n, err)
		}
	}
	if string(b.Bytes()) != "abcde" || !b.exceeded() || fired != 1 {
		t.Errorf("bytes=%q exceeded=%v fired=%d", b.Bytes(), b.exceeded(), fired)
	}
	u := newBoundedBuffer(0, nil)
	_, _ = u.Write([]byte("unbounded"))
	if string(u.Bytes()) != "unbounded" || u.exceeded() {
		t.Errorf("unbounded buffer = %q", u.Bytes())
	}
}
