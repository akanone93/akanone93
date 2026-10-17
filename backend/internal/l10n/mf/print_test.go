// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package mf

import "testing"

// Verifies: I18N-001, I18N-002.
func TestPrintRoundTrips(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"Hello",
		"Hi {name}, it''s {when, date, short}",
		"{n, plural, offset:1 one {# thing} other {# things}}",
		"{g, select, f {She} other {They}} left",
		"{n, selectordinal, one {#st} other {#th}}",
		"Braces '{' and '}' stay text",
		"{n, plural, other {'#' is not a count}}",
	} {
		m, err := Parse(src)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		printed := m.String()
		again, err := Parse(printed)
		if err != nil {
			t.Fatalf("%q printed as %q does not parse: %v", src, printed, err)
		}
		if got := again.String(); got != printed {
			t.Errorf("%q: printing is not stable: %q then %q", src, printed, got)
		}
		if string(again.Encode()) != string(m.Encode()) {
			t.Errorf("%q printed as %q changes the message", src, printed)
		}
	}
}
