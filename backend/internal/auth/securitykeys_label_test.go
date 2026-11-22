// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: SEC-100.
// A security key's name is required and plain text, so the keys of an
// account can be told apart when one is lost or removed (#232).
func TestSecurityKeyNameIsRequiredAndPlain_SEC_100(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"YubiKey", "YubiKey", true},
		{"  Laptop  ", "Laptop", true},
		{"مفتاح العمل", "مفتاح العمل", true},
		{strings.Repeat("ß", 100), strings.Repeat("ß", 100), true},
		{"", "", false},
		{"   ", "", false},
		{strings.Repeat("a", 101), "", false},
		{"bell\x07key", "", false},
		{"line\nbreak", "", false},
		{"bad\xffbyte", "", false},
		{"key\u202Egnp.exe", "", false},
		{"\u2067work\u2069", "", false},
		{"\u200Fمفتاح", "\u200Fمفتاح", true},
	} {
		got, err := factorLabel(tc.in)
		if tc.ok {
			if err != nil || got != tc.want {
				t.Errorf("factorLabel(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
			}
			continue
		}
		if c, ok := plxerr.CodeOf(err); !ok || c != plxerr.InvalidFormat {
			t.Errorf("factorLabel(%q) = %q, %v; want an InvalidFormat refusal", tc.in, got, err)
		}
	}
}
