// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"
)

// Verifies: QA-073 (catalogue-check flags and exit codes).
func TestRun(t *testing.T) {
	root := "../../.."
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
	}{
		{"report exits zero", []string{"-root", root, "-phase", "P8", "-report"}, exitOK, "WIDGET TabBar:"},
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
