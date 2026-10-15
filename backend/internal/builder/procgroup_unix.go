// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package builder

import (
	"os"
	"os/exec"
	"syscall"
)

// configureGroup puts the child in its own process group and makes the
// context's cancellation kill the whole group, so a build cannot leave
// compilers behind (FN-016).
func configureGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		if err := syscall.Kill(-c.Process.Pid, syscall.SIGKILL); err != nil {
			return c.Process.Kill()
		}
		return nil
	}
}

// exitSignal returns the number of the signal that ended the process, or 0.
func exitSignal(ps *os.ProcessState) int {
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() { //nolint:misspell // the method name is the standard library's.
		return int(ws.Signal())
	}
	return 0
}
