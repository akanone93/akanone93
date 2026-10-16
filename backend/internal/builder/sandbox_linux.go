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
	"unsafe"
)

// LinuxSandbox runs each child in new user, network, mount and PID namespaces:
// the child sees no network interface but loopback, cannot signal processes
// outside the sandbox and is killed with its whole group when the wall time
// ends or the builder dies (FN-016, ADR-0063). The CPU-time and memory limits
// are in force before the tool's first instruction: /bin/sh sets them and
// replaces itself with the tool (limitedCommand), and prlimit(2) asserts them
// again once the child exists. It is safe for concurrent use.
type LinuxSandbox struct{}

// Run implements Sandbox. It returns ErrSandboxUnavailable when the kernel
// refuses the namespaces.
func (LinuxSandbox) Run(ctx context.Context, cmd Command) (Result, error) {
	return runChild(ctx, limitedCommand(cmd), childHooks{prepare: isolate, afterStart: applyLimits, classifyStart: classifyStartError})
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

// applyLimits sets the CPU-time and address-space limits of the started child
// again; limitedCommand has already set them before the tool started.
func applyLimits(c *exec.Cmd, l Limits) error {
	pid := c.Process.Pid
	if l.CPUTime > 0 {
		secs := uint64((l.CPUTime + time.Second - 1) / time.Second)
		// The soft limit raises SIGXCPU; a second later the hard limit kills.
		if err := setRlimit(pid, syscall.RLIMIT_CPU, syscall.Rlimit{Cur: secs, Max: secs + 1}); err != nil {
			return err
		}
	}
	if l.Memory > 0 {
		if err := setRlimit(pid, syscall.RLIMIT_AS, syscall.Rlimit{Cur: l.Memory, Max: l.Memory}); err != nil {
			return err
		}
	}
	return nil
}

// setRlimit calls prlimit64(2) on a child.
func setRlimit(pid, resource int, lim syscall.Rlimit) error {
	_, _, errno := syscall.RawSyscall6(syscall.SYS_PRLIMIT64, uintptr(pid), uintptr(resource),
		uintptr(unsafe.Pointer(&lim)), 0, 0, 0) //nolint:gosec // G103: the pointer is to a stack value that outlives this one syscall.
	if errno != 0 {
		return fmt.Errorf("prlimit64 resource %d: %w", resource, errno)
	}
	return nil
}
