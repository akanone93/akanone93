// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package builder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// LinuxSandbox runs each child in new user, network, mount and PID namespaces:
// the child sees no network interface but loopback, cannot signal processes
// outside the sandbox and is killed with its whole group when the wall time
// ends or the builder dies (FN-016, ADR-0063). It sees a private view of the
// filesystem — the system directories, Command.Readable read-only and
// Command.Writable writable, nothing else — and Landlock restricts it to the
// same, so no build can reach another build's files or the builder's own
// (build log #164). The helper (SandboxHelperArg) builds the view, applies
// Landlock and the CPU-time and memory limits, and replaces itself with the
// tool through /bin/sh, which sets the limits again (limitedCommand). It is
// safe for concurrent use.
type LinuxSandbox struct {
	// Helper is the executable that runs as the sandbox helper: the
	// program's own, which calls RunSandboxHelper when its first argument
	// is SandboxHelperArg.
	Helper string
}

// Run implements Sandbox. It returns ErrSandboxUnavailable when the kernel
// refuses the namespaces or offers no Landlock.
func (s LinuxSandbox) Run(ctx context.Context, cmd Command) (Result, error) {
	wrapped, err := helperCommand(s.Helper, cmd)
	if err != nil {
		return Result{}, err
	}
	res, err := runChild(ctx, wrapped, childHooks{prepare: isolate, classifyStart: classifyStartError})
	if err == nil && helperRefused(res) {
		return res, fmt.Errorf("%w: %s", ErrSandboxUnavailable, strings.TrimSpace(string(res.Stderr)))
	}
	return res, err
}

// limitedCommand runs cmd through /bin/sh, which sets its CPU-time and
// address-space limits and then execs the tool, so the tool starts under
// them. Limits set only after the start race the tool: Bun, which runs
// wasm-opt, reserves tens of GiB of address space as it starts unless a
// limit is already in force, and failed every allocation after a late one
// (CI runs 38782704836, 38893983660). The soft CPU limit raises SIGXCPU; a
// second later the hard limit kills. A command without those limits runs
// as it is.
func limitedCommand(cmd Command) Command {
	var set []string
	if cmd.Limits.CPUTime > 0 {
		secs := uint64((cmd.Limits.CPUTime + time.Second - 1) / time.Second)
		// Soft first: a hard limit below the current soft one is refused.
		set = append(set, fmt.Sprintf("ulimit -S -t %d", secs), fmt.Sprintf("ulimit -H -t %d", secs+1))
	}
	if cmd.Limits.Memory > 0 {
		set = append(set, fmt.Sprintf("ulimit -v %d", (cmd.Limits.Memory+1023)/1024))
	}
	if len(set) == 0 {
		return cmd
	}
	script := strings.Join(set, " && ") + ` && exec "$0" "$@"`
	cmd.Args = append([]string{"-c", script, cmd.Path}, cmd.Args...)
	cmd.Path = "/bin/sh"
	return cmd
}

// isolate adds the namespaces to the child's process attributes.
func isolate(c *exec.Cmd) {
	uid, gid := os.Getuid(), os.Getgid()
	c.SysProcAttr.Cloneflags = syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET | syscall.CLONE_NEWNS | syscall.CLONE_NEWPID
	c.SysProcAttr.UidMappings = []syscall.SysProcIDMap{{ContainerID: uid, HostID: uid, Size: 1}}
	c.SysProcAttr.GidMappings = []syscall.SysProcIDMap{{ContainerID: gid, HostID: gid, Size: 1}}
	c.SysProcAttr.GidMappingsEnableSetgroups = false
	// The child keeps its own user ID, so it would lose the capabilities of
	// its new user namespace when it starts the helper. The helper needs two
	// of them, in its own namespaces only, to build the view and then drop
	// every capability before the tool starts (dropCapabilities).
	c.SysProcAttr.AmbientCaps = []uintptr{capSysAdmin, capSetPCap}
	c.SysProcAttr.Pdeathsig = syscall.SIGKILL
}

// classifyStartError turns the errors the kernel gives for a refused namespace
// into ErrSandboxUnavailable; other start errors pass through.
func classifyStartError(err error) error {
	switch {
	case errors.Is(err, syscall.EPERM), errors.Is(err, syscall.EINVAL),
		errors.Is(err, syscall.ENOSPC), errors.Is(err, syscall.EUSERS):
		return fmt.Errorf("%w: %w", ErrSandboxUnavailable, err)
	default:
		return err
	}
}
