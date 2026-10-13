// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// Verifies: FN-013.
// The runner holds no database, storage or signing capability (L-4): none of
// those packages is among its dependencies, however indirect.
func TestRunnerImportsNoDataOrSigningPackage_FN_013(t *testing.T) {
	t.Parallel()
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go tool is not on PATH")
	}
	out, err := exec.CommandContext(context.Background(), goTool, "list", "-deps", ".").Output() //nolint:gosec // the go tool found on PATH, fixed arguments
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	forbidden := []string{
		"/backend/internal/storage",
		"/backend/internal/signing",
		"/backend/internal/pkcs11",
		"github.com/jackc/pgx",
		"github.com/lib/pq",
		"database/sql",
	}
	deps := strings.Fields(string(out))
	if len(deps) < 10 {
		t.Fatalf("go list returned %d packages; the check would prove nothing", len(deps))
	}
	for _, dep := range deps {
		for _, bad := range forbidden {
			if strings.Contains(dep, bad) {
				t.Errorf("fnrunner depends on %s (forbidden: %s)", dep, bad)
			}
		}
	}
}
