// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package builder

import (
	"os"
	"os/exec"
)

// configureGroup kills the child on cancellation; process groups are not
// available on this platform, so grandchildren may outlive a timeout. Only the
// development setting without a sandbox runs here.
func configureGroup(c *exec.Cmd) {
	c.Cancel = func() error { return c.Process.Kill() }
}

// exitSignal returns 0: this platform reports no signals.
func exitSignal(*os.ProcessState) int { return 0 }
