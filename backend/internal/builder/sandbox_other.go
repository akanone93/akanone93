// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package builder

import (
	"context"
	"fmt"
	"os"
)

// LinuxSandbox is the namespace sandbox; it exists only on Linux. On this
// platform every Run returns ErrSandboxUnavailable, so the builder refuses to
// build unless the caller chose [NoSandbox] explicitly. It is safe for
// concurrent use.
type LinuxSandbox struct {
	// Helper is as on Linux; it is not used here.
	Helper string
}

// Run implements Sandbox.
func (LinuxSandbox) Run(context.Context, Command) (Result, error) {
	return Result{}, ErrSandboxUnavailable
}

// RunSandboxHelper exists only to be called; on this platform there is no
// sandbox to prepare, and it fails.
func RunSandboxHelper([]string) int {
	fmt.Fprintln(os.Stderr, "plux-sandbox: the sandbox needs Linux")
	return 2
}
