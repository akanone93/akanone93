// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureRoot writes a repository whose appendix lists a widget with no
// descriptor, so the check has exactly one finding.
func fixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	spec := "## Appendix C — Widget Catalogue\n| `Ghost` | P3 |\n## Appendix D — Built-in Action Catalogue\n## Appendix E\n"
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "requirements.md"), []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// Verifies: QA-073 (catalogue-check flags and exit codes).
func TestRun(t *testing.T) {
	root := fixtureRoot(t)
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
	}{
		{"report exits zero", []string{"-root", root, "-phase", "P8", "-report"}, exitOK, "WIDGET Ghost: missing descriptor"},
		{"findings fail", []string{"-root", root, "-phase", "P8"}, exitFindings, " findings"},
		{"missing flags", nil, exitError, ""},
		{"bad phase", []string{"-root", root, "-phase", "8"}, exitError, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			if code := run(tc.args, &out, &errb); code != tc.wantCode {
				t.Fatalf("code = %d, want %d (%s)", code, tc.wantCode, errb.String())
			}
			if !strings.Contains(out.String(), tc.wantOut) {
				t.Errorf("output lacks %q", tc.wantOut)
			}
		})
	}
}
