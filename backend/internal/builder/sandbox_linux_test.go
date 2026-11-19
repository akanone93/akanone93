// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package builder

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// requireNamespaces skips the test where the kernel refuses the namespaces.
func requireNamespaces(t *testing.T) {
	t.Helper()
	if _, err := testSandbox(t).Run(t.Context(), inScratch(t, shell("true", Limits{}))); err != nil {
		if errors.Is(err, ErrSandboxUnavailable) {
			t.Skipf("user and network namespaces are unavailable here: %v", err)
		}
		t.Fatal(err)
	}
}

// Verifies: FN-016.
func TestLinuxSandboxShowsOnlyLoopback_FN_016(t *testing.T) {
	requireNamespaces(t)
	s := testSandbox(t)
	res, err := s.Run(t.Context(), inScratch(t, shell("tail -n +3 /proc/net/dev | cut -d: -f1 | tr -d ' '", Limits{Wall: 20 * time.Second})))
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
	s := testSandbox(t)
	start := time.Now()
	res, err := s.Run(t.Context(), inScratch(t, shell("sleep 30 & sleep 30", Limits{Wall: 300 * time.Millisecond})))
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
	s := testSandbox(t)
	// The shell waits so the limit is in place before it allocates.
	script := "sleep 0.5; x=$(head -c 200000000 /dev/zero | tr '\\0' a); echo ${#x}"
	res, err := s.Run(t.Context(), inScratch(t, shell(script, Limits{Memory: 96 << 20, Wall: 30 * time.Second})))
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
	s := testSandbox(t)
	res, err := s.Run(t.Context(), inScratch(t, shell("while :; do :; done", Limits{CPUTime: time.Second, Wall: 30 * time.Second})))
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
	s := testSandbox(t)
	res, err := s.Run(t.Context(), inScratch(t, shell("id -u", Limits{})))
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

// Verifies: FN-016.
// The helper refuses a specification it cannot read, or one without a
// writable directory, before it changes anything.
func TestSandboxHelperRefusesABadSpecification_FN_016(t *testing.T) {
	for _, args := range [][]string{nil, {"{"}, {`{"path":"/bin/true"}`}} {
		if code := RunSandboxHelper(args); code != 2 {
			t.Errorf("RunSandboxHelper(%q) = %d; want 2", args, code)
		}
	}
	if _, err := (LinuxSandbox{}).Run(t.Context(), inScratch(t, shell("true", Limits{}))); err == nil {
		t.Error("a sandbox without a helper ran a child")
	}
	if _, err := testSandbox(t).Run(t.Context(), shell("true", Limits{})); err == nil {
		t.Error("a child without a writable directory ran")
	}
}

// Verifies: FN-016.
// A sandboxed child sees only its view: the system directories and the
// readable directories, read-only, its own scratch directory, writable, and
// nothing else — not another build's scratch directory, not the builder's
// files. Its temporary files go to its scratch directory (build log #164).
func TestLinuxSandboxShowsOnlyItsView_FN_016(t *testing.T) {
	requireNamespaces(t)
	s := testSandbox(t)
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "secret"), []byte("another build's file"), 0o600); err != nil {
		t.Fatal(err)
	}
	readable := t.TempDir()
	if err := os.WriteFile(filepath.Join(readable, "tool"), []byte("a tool"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := strings.Join([]string{
		"cat " + filepath.Join(other, "secret") + " 2>/dev/null && echo SEEN-OTHER",
		"echo x > " + filepath.Join(other, "planted") + " 2>/dev/null && echo WROTE-OTHER",
		"cat " + filepath.Join(readable, "tool"),
		"echo x > " + filepath.Join(readable, "changed") + " 2>/dev/null && echo WROTE-READABLE",
		"echo x > /usr/planted 2>/dev/null && echo WROTE-USR",
		"echo kept > own && cat own",
		"echo masked=$(cat /proc/timer_list /proc/keys /proc/kcore 2>/dev/null | wc -c)",
		"echo first=$(tr '\\0' '\\n' < /proc/1/cmdline | head -n 1) processes=$(ls -d /proc/[0-9]* | wc -l)",
		"echo $TMPDIR",
		"ls /",
	}, "; ")
	c := inScratch(t, shell(script, Limits{Wall: 20 * time.Second}))
	c.Readable = []string{readable}
	res, err := s.Run(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.Stdout)
	for _, leak := range []string{"SEEN-OTHER", "WROTE-OTHER", "WROTE-READABLE", "WROTE-USR"} {
		if strings.Contains(out, leak) {
			t.Errorf("the child could %s:\n%s", leak, out)
		}
	}
	// The shell is process 1 of the child's PID namespace, and /proc holds
	// it and the few commands of the script, nothing of the machine.
	if m := regexp.MustCompile(`first=(\S+) processes=(\d+)`).FindStringSubmatch(out); m == nil || m[1] != "/bin/sh" || len(m[2]) != 1 || m[2] > "4" {
		t.Errorf("the child's /proc shows processes outside its PID namespace:\n%s", out)
	}
	for _, want := range []string{"a tool", "kept", "masked=0\n", filepath.Join(c.Dir, "tmp")} {
		if !strings.Contains(out, want) {
			t.Errorf("the child's output lacks %q (stderr %q):\n%s", want, res.Stderr, out)
		}
	}
	if _, err := os.Stat(filepath.Join(other, "planted")); err == nil {
		t.Error("a file was written outside the child's scratch directory")
	}
	tmp := filepath.Join(c.Dir, "tmp")
	at := strings.LastIndex(out, tmp)
	if at < 0 {
		return
	}
	for _, entry := range strings.Fields(out[at+len(tmp):]) {
		switch entry {
		case "bin", "dev", "etc", "lib", "lib32", "lib64", "libx32", "proc", "sbin", "usr", strings.Split(strings.TrimPrefix(c.Dir, "/"), "/")[0], strings.Split(strings.TrimPrefix(readable, "/"), "/")[0]:
		default:
			t.Errorf("the view's root holds %q", entry)
		}
	}
}

// Verifies: FN-016.
// Landlock binds the child too: on a kernel with Landlock network rules no
// TCP connection is allowed, even on loopback, which the network namespace
// alone would let through.
func TestLinuxSandboxLandlockRefusesTCP_FN_016(t *testing.T) {
	requireNamespaces(t)
	abi, _, errno := syscall.Syscall(sysLandlockCreateRuleset, 0, 0, landlockCreateRulesetVersion)
	if errno != 0 || abi < 4 {
		t.Skipf("Landlock network rules need ABI 4 (have %d, %v)", abi, errno)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is needed to open a socket")
	}
	if python, err = filepath.EvalSymlinks(python); err != nil || !strings.HasPrefix(python, "/usr/") {
		t.Skipf("python3 is outside /usr (%s, %v), so the view does not have it", python, err)
	}
	res, err := testSandbox(t).Run(t.Context(), inScratch(t, Command{
		Path: python, Args: []string{"-I", "-c", "import socket\ntry:\n socket.create_connection(('127.0.0.1', 9), 2)\nexcept PermissionError:\n print('refused by Landlock')\nexcept OSError as e:\n print('network', e)"},
		Env: []string{"PATH=/usr/bin:/bin"}, Limits: Limits{Wall: 20 * time.Second},
	}))
	if err != nil || !strings.Contains(string(res.Stdout), "refused by Landlock") {
		t.Errorf("a TCP connection was not refused by Landlock: %q %q %v", res.Stdout, res.Stderr, err)
	}
}

// TestLinuxSandboxToolHoldsNoCapabilities_FN_016 checks that the tool starts
// with every capability set empty, in its own namespaces too: the helper
// drops the two it needed to build the view, and a builder running as root
// does not hand the tool the capabilities of root.
// Verifies: FN-016.
func TestLinuxSandboxToolHoldsNoCapabilities_FN_016(t *testing.T) {
	requireNamespaces(t)
	res, err := testSandbox(t).Run(t.Context(), inScratch(t, Command{
		Path: "/bin/cat", Args: []string{"/proc/self/status"}, Env: []string{"PATH=/usr/bin:/bin"},
		Limits: Limits{Wall: 20 * time.Second},
	}))
	if err != nil {
		t.Fatal(err)
	}
	status := string(res.Stdout)
	for _, set := range []string{"CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"} {
		if !strings.Contains(status, set+":\t0000000000000000\n") {
			t.Errorf("the tool holds capabilities in %s (exit %d, stderr %q):\n%s", set, res.ExitCode, res.Stderr, status)
		}
	}
	if !strings.Contains(status, "NoNewPrivs:\t1\n") {
		t.Errorf("the tool can gain privileges on exec:\n%s", status)
	}
}

// mountScript makes a new user and mount namespace, in which the process
// holds every capability, and mounts a tmpfs on the working directory; only
// Landlock stands in the way of the mount.
const mountScript = `import ctypes, os
libc = ctypes.CDLL(None, use_errno=True)
os.unshare(os.CLONE_NEWUSER | os.CLONE_NEWNS)
if libc.mount(b"tmpfs", b".", b"tmpfs", 0, None) == 0:
    print("mounted")
else:
    print("mount refused", os.strerror(ctypes.get_errno()))
`

// TestLinuxSandboxToolCannotMount_FN_016 checks that a tool cannot change its
// view: even in a user namespace of its own, where it holds every capability,
// Landlock refuses mount(2) before any filesystem code runs. So the mount
// calls the builder's seccomp profile allows serve only the helper, which
// builds the view before it restricts itself.
// Verifies: FN-016.
func TestLinuxSandboxToolCannotMount_FN_016(t *testing.T) {
	requireNamespaces(t)
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is needed to call mount(2)")
	}
	if python, err = filepath.EvalSymlinks(python); err != nil || !strings.HasPrefix(python, "/usr/") {
		t.Skipf("python3 is outside /usr (%s, %v), so the view does not have it", python, err)
	}
	control := exec.CommandContext(t.Context(), python, "-I", "-c", mountScript) //nolint:gosec // G204: the test's own interpreter and script.
	control.Dir = t.TempDir()
	if out, err := control.Output(); err != nil || !strings.Contains(string(out), "mounted") {
		t.Skipf("this machine refuses the mount even without the sandbox (%q, %v), so the test would show nothing", out, err)
	}
	res, err := testSandbox(t).Run(t.Context(), inScratch(t, Command{
		Path: python, Args: []string{"-I", "-c", mountScript},
		Env: []string{"PATH=/usr/bin:/bin"}, Limits: Limits{Wall: 20 * time.Second},
	}))
	if err != nil || !strings.Contains(string(res.Stdout), "mount refused Operation not permitted") {
		t.Errorf("a tool changed its view: %q %q %v", res.Stdout, res.Stderr, err)
	}
}

// TestLinuxSandboxLimitsHoldFromTheStart checks that the tool's first
// instruction already runs under its memory and CPU-time limits: Bun reserves
// tens of GiB of address space as it starts unless a limit is in force then,
// and a limit set later made every allocation fail.
// Verifies: FN-016.
func TestLinuxSandboxLimitsHoldFromTheStart_FN_016(t *testing.T) {
	requireNamespaces(t)
	s := testSandbox(t)
	res, err := s.Run(t.Context(), inScratch(t, Command{
		Path: "/bin/cat", Args: []string{"/proc/self/limits"}, Env: []string{"PATH=/usr/bin:/bin"},
		Limits: Limits{Memory: 96 << 20, CPUTime: 3 * time.Second, Wall: 20 * time.Second},
	}))
	if err != nil {
		t.Fatal(err)
	}
	limits := string(res.Stdout)
	for _, want := range []string{"Max address space         100663296            100663296", "Max cpu time              3                    4"} {
		if !strings.Contains(limits, want) {
			t.Errorf("the tool did not start under %q (exit %d, stderr %q):\n%s", want, res.ExitCode, res.Stderr, limits)
		}
	}
	res, err = s.Run(t.Context(), inScratch(t, Command{Path: "/bin/echo", Args: []string{"a b", "$HOME"}, Limits: Limits{Memory: 96 << 20}}))
	if err != nil || string(res.Stdout) != "a b $HOME\n" {
		t.Errorf("the arguments did not reach the tool unchanged: %q, %v", res.Stdout, err)
	}
}

// TestLimitedCommandSetsTheLimitsBeforeTheTool checks the wrapper itself, as
// the race it closes cannot be forced: the shell sets the soft CPU limit,
// then the hard one and the address space, then replaces itself with the
// tool and its arguments; a command without those limits is unchanged.
// Verifies: FN-016.
func TestLimitedCommandSetsTheLimitsBeforeTheTool_FN_016(t *testing.T) {
	t.Parallel()
	cmd := Command{Path: "/usr/bin/bun", Args: []string{"wasm-opt", "--version"}, Limits: Limits{CPUTime: 1500 * time.Millisecond, Memory: 1 << 30, Wall: time.Minute}}
	got := limitedCommand(cmd)
	want := []string{"-c", `ulimit -S -t 2 && ulimit -H -t 3 && ulimit -v 1048576 && exec "$0" "$@"`, "/usr/bin/bun", "wasm-opt", "--version"}
	if got.Path != "/bin/sh" || strings.Join(got.Args, "\x00") != strings.Join(want, "\x00") || got.Limits != cmd.Limits {
		t.Errorf("limitedCommand = %q %q", got.Path, got.Args)
	}
	plain := Command{Path: "/bin/true", Limits: Limits{Wall: time.Minute}}
	if got := limitedCommand(plain); got.Path != plain.Path || len(got.Args) != 0 {
		t.Errorf("a command without CPU or memory limits was wrapped: %q %q", got.Path, got.Args)
	}
}
