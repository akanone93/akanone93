// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package builder

import (
	"errors"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// requireNamespaces skips the test where the kernel refuses the namespaces.
func requireNamespaces(t *testing.T) {
	t.Helper()
	if _, err := (LinuxSandbox{}).Run(t.Context(), shell("true", Limits{})); err != nil {
		if errors.Is(err, ErrSandboxUnavailable) {
			t.Skipf("user and network namespaces are unavailable here: %v", err)
		}
		t.Fatal(err)
	}
}

// Verifies: FN-016.
func TestLinuxSandboxShowsOnlyLoopback_FN_016(t *testing.T) {
	requireNamespaces(t)
	s := LinuxSandbox{}
	res, err := s.Run(t.Context(), shell("tail -n +3 /proc/net/dev | cut -d: -f1 | tr -d ' '", Limits{Wall: 20 * time.Second}))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(res.Stdout)); got != "lo" {
		t.Errorf("interfaces in the sandbox = %q; want only lo (stderr %q)", got, res.Stderr)
	}
}

// Verifies: FN-016.
func TestLinuxSandboxKillsTheGroupAtTheWallTime_FN_016(t *testing.T) {
	requireNamespaces(t)
	s := LinuxSandbox{}
	start := time.Now()
	res, err := s.Run(t.Context(), shell("sleep 30 & sleep 30", Limits{Wall: 300 * time.Millisecond}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.TimedOut || time.Since(start) > 10*time.Second {
		t.Errorf("TimedOut=%v after %s", res.TimedOut, time.Since(start))
	}
}

// Verifies: FN-016.
func TestLinuxSandboxLimitsMemory_FN_016(t *testing.T) {
	requireNamespaces(t)
	s := LinuxSandbox{}
	// The shell waits so the limit is in place before it allocates.
	script := "sleep 0.5; x=$(head -c 200000000 /dev/zero | tr '\\0' a); echo ${#x}"
	res, err := s.Run(t.Context(), shell(script, Limits{Memory: 96 << 20, Wall: 30 * time.Second}))
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode == 0 && strings.TrimSpace(string(res.Stdout)) == "200000000" {
		t.Errorf("the child allocated 200 MB under a 96 MiB limit")
	}
}

// Verifies: FN-016.
func TestLinuxSandboxLimitsCPUTime_FN_016(t *testing.T) {
	requireNamespaces(t)
	s := LinuxSandbox{}
	res, err := s.Run(t.Context(), shell("while :; do :; done", Limits{CPUTime: time.Second, Wall: 30 * time.Second}))
	if err != nil {
		t.Fatal(err)
	}
	// The sandbox's first process ignores SIGXCPU, so the hard limit kills it.
	if res.TimedOut || (res.Signal != int(syscall.SIGXCPU) && res.Signal != int(syscall.SIGKILL)) {
		t.Errorf("signal = %d, TimedOut = %v; want SIGXCPU or SIGKILL", res.Signal, res.TimedOut)
	}
}

// Verifies: FN-016.
func TestLinuxSandboxRunsAsTheSameUser_FN_016(t *testing.T) {
	requireNamespaces(t)
	s := LinuxSandbox{}
	res, err := s.Run(t.Context(), shell("id -u", Limits{}))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(res.Stdout)); got != strconv.Itoa(syscall.Getuid()) {
		t.Errorf("uid in the sandbox = %q", got)
	}
}

func TestClassifyStartErrorMarksRefusedNamespaces(t *testing.T) {
	for _, e := range []error{syscall.EPERM, syscall.EINVAL, syscall.ENOSPC, syscall.EUSERS} {
		if err := classifyStartError(e); !errors.Is(err, ErrSandboxUnavailable) {
			t.Errorf("%v not classified as unavailable", e)
		}
	}
	if err := classifyStartError(syscall.ENOENT); errors.Is(err, ErrSandboxUnavailable) {
		t.Error("ENOENT must stay a plain start error")
	}
}

func TestApplyLimitsFailsOnAGoneProcess(t *testing.T) {
	if err := setRlimit(1<<30, syscall.RLIMIT_AS, syscall.Rlimit{Cur: 1, Max: 1}); err == nil {
		t.Error("expected prlimit to fail for a process that does not exist")
	}
}

// TestLinuxSandboxLimitsHoldFromTheStart checks that the tool's first
// instruction already runs under its memory and CPU-time limits: Bun reserves
// tens of GiB of address space as it starts unless a limit is in force then,
// and a limit set later made every allocation fail.
// Verifies: FN-016.
func TestLinuxSandboxLimitsHoldFromTheStart_FN_016(t *testing.T) {
	requireNamespaces(t)
	s := LinuxSandbox{}
	res, err := s.Run(t.Context(), Command{
		Path: "/bin/cat", Args: []string{"/proc/self/limits"}, Env: []string{"PATH=/usr/bin:/bin"},
		Limits: Limits{Memory: 96 << 20, CPUTime: 3 * time.Second, Wall: 20 * time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	limits := string(res.Stdout)
	for _, want := range []string{"Max address space         100663296            100663296", "Max cpu time              3                    4"} {
		if !strings.Contains(limits, want) {
			t.Errorf("the tool did not start under %q:\n%s", want, limits)
		}
	}
	res, err = s.Run(t.Context(), Command{Path: "/bin/echo", Args: []string{"a b", "$HOME"}, Limits: Limits{Memory: 96 << 20}})
	if err != nil || string(res.Stdout) != "a b $HOME\n" {
		t.Errorf("the arguments did not reach the tool unchanged: %q, %v", res.Stdout, err)
	}
}

// TestLimitedCommandSetsTheLimitsBeforeTheTool checks the wrapper itself, as
// the race it closes cannot be forced: the shell sets the hard CPU limit,
// then the soft one and the address space, then replaces itself with the
// tool and its arguments; a command without those limits is unchanged.
// Verifies: FN-016.
func TestLimitedCommandSetsTheLimitsBeforeTheTool_FN_016(t *testing.T) {
	t.Parallel()
	cmd := Command{Path: "/usr/bin/bun", Args: []string{"wasm-opt", "--version"}, Limits: Limits{CPUTime: 1500 * time.Millisecond, Memory: 1 << 30, Wall: time.Minute}}
	got := limitedCommand(cmd)
	want := []string{"-c", `ulimit -H -t 3 && ulimit -S -t 2 && ulimit -v 1048576 && exec "$0" "$@"`, "/usr/bin/bun", "wasm-opt", "--version"}
	if got.Path != "/bin/sh" || strings.Join(got.Args, "\x00") != strings.Join(want, "\x00") || got.Limits != cmd.Limits {
		t.Errorf("limitedCommand = %q %q", got.Path, got.Args)
	}
	plain := Command{Path: "/bin/true", Limits: Limits{Wall: time.Minute}}
	if got := limitedCommand(plain); got.Path != plain.Path || len(got.Args) != 0 {
		t.Errorf("a command without CPU or memory limits was wrapped: %q %q", got.Path, got.Args)
	}
}
