// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package builderrole

import (
	"os/exec"
	"strings"
	"testing"
)

// Verifies: FN-016.
// The builder role holds no database, object-store or signing capability
// (L-4): its dependency graph reaches none of them, so no code path of the
// role could use one.
func TestBuilderRoleImportsNoDataOrSigningPackage_FN_016(t *testing.T) {
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", ".").Output() //nolint:gosec // G204: a fixed command.
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	forbidden := []string{
		"/backend/internal/storage", "/backend/internal/signing", "/backend/internal/auth", "/backend/internal/tenancy",
		"/backend/internal/function", "/backend/internal/release", "/backend/internal/jobs",
		"github.com/jackc/pgx", "github.com/riverqueue", "github.com/aws/aws-sdk-go",
	}
	for _, pkg := range strings.Fields(string(out)) {
		for _, bad := range forbidden {
			if strings.Contains(pkg, bad) {
				t.Errorf("the builder role depends on %s, which is forbidden (L-4)", pkg)
			}
		}
	}
}
