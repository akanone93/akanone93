// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package builder

import (
	"context"
)

// LinuxSandbox is the namespace sandbox; it exists only on Linux. On this
// platform every Run returns ErrSandboxUnavailable, so the builder refuses to
// build unless the caller chose [NoSandbox] explicitly. It is safe for
// concurrent use.
type LinuxSandbox struct{}

// Run implements Sandbox.
func (LinuxSandbox) Run(context.Context, Command) (Result, error) {
	return Result{}, ErrSandboxUnavailable
}
