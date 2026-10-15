// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package egress

import (
	"slices"
	"strings"
	"testing"
)

// Verifies: FN-017.
func TestMatchAllowlist_FN_017(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		patterns []string
		host     string
		want     bool
	}{
		{"exact", []string{"api.example.com"}, "api.example.com", true},
		{"exact is not a suffix match", []string{"example.com"}, "api.example.com", false},
		{"exact is not a prefix match", []string{"api.example.com"}, "example.com", false},
		{"wildcard one label", []string{"*.example.com"}, "api.example.com", true},
		{"wildcard many labels", []string{"*.example.com"}, "a.b.c.example.com", true},
		{"wildcard excludes the apex", []string{"*.example.com"}, "example.com", false},
		{"wildcard needs a label boundary", []string{"*.example.com"}, "evilexample.com", false},
		{"wildcard is not a prefix match", []string{"*.example.com"}, "example.com.evil.org", false},
		{"second pattern matches", []string{"a.test", "*.example.com"}, "x.example.com", true},
		{"no patterns", nil, "example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := matchAllowlist(tc.patterns, tc.host); got != tc.want {
				t.Errorf("matchAllowlist(%v, %q) = %v, want %v", tc.patterns, tc.host, got, tc.want)
			}
		})
	}
}

// Verifies: FN-017.
func TestParseAllowlist_FN_017(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		values []string
		want   []string
	}{
		{"case and trailing dot", []string{"API.Example.COM., *.Example.com"}, []string{"api.example.com", "*.example.com"}},
		{"several header lines", []string{"a.test", "b.test,c.test"}, []string{"a.test", "b.test", "c.test"}},
		{"non-ASCII is refused", []string{"bücher.example, ok.test"}, []string{"ok.test"}},
		{"already encoded IDNA is accepted", []string{"xn--bcher-kva.example"}, []string{"xn--bcher-kva.example"}},
		{"a bare star is dropped", []string{"*, *.*, ok.test"}, []string{"ok.test"}},
		{"a star inside a label is dropped", []string{"a*.example.com, ok.test"}, []string{"ok.test"}},
		{"an IP address is dropped", []string{"10.0.0.1, ::1, *.10.0.0.1, ok.test"}, []string{"ok.test"}},
		{"empty entries", []string{",, ,"}, nil},
		{"empty labels", []string{"a..b, .a, ok.test"}, []string{"ok.test"}},
		{"spaces inside", []string{"a b.test"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := parseAllowlist(tc.values); !slices.Equal(got, tc.want) {
				t.Errorf("parseAllowlist(%q) = %q, want %q", tc.values, got, tc.want)
			}
		})
	}
}

// Verifies: FN-017.
func TestParseAllowlistIsBounded_FN_017(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	for i := range maxPatterns + 50 {
		b.WriteString("h")
		b.WriteString(strings.Repeat("a", i%5))
		b.WriteString(".test,")
	}
	if got := parseAllowlist([]string{b.String()}); len(got) != maxPatterns {
		t.Errorf("len = %d, want %d", len(got), maxPatterns)
	}
}

// Verifies: FN-017.
func TestNormalizeHost_FN_017(t *testing.T) {
	t.Parallel()
	good := map[string]string{"Example.COM": "example.com", "example.com.": "example.com", "a_b.example": "a_b.example"}
	for in, want := range good {
		if got, ok := normalizeHost(in); !ok || got != want {
			t.Errorf("normalizeHost(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"", ".", "a..b", "-.", "a b", "bücher.example", "a:b", strings.Repeat("a", 64) + ".test", strings.Repeat("a.", 130) + "test"} {
		if got, ok := normalizeHost(in); ok {
			t.Errorf("normalizeHost(%q) = %q, want a refusal", in, got)
		}
	}
}

// Verifies: FN-017.
func TestLooksLikeAddress_FN_017(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"127.0.0.1", "10.1.2.3", "2130706433", "0x7f.1", "127.1", "::1", "fe80::1%eth0", "::ffff:10.0.0.1", "example.0x10"} {
		if !looksLikeAddress(in) {
			t.Errorf("looksLikeAddress(%q) = false", in)
		}
	}
	for _, in := range []string{"example.com", "localhost", "a1.b2", "xn--bcher-kva.example", "10.example.com"} {
		if looksLikeAddress(in) {
			t.Errorf("looksLikeAddress(%q) = true", in)
		}
	}
}
