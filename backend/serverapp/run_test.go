// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package serverapp

import (
	"context"
	"io"
)

// run is the open-source core's server, the way cmd/plux-server runs it.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return Run(ctx, args, stdout, stderr, Options{})
}
