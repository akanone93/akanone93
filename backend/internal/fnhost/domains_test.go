// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import "testing"

// Verifies: FN-012.
func TestDomainAllowed_FN_012(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		allow []string
		host  string
		want  bool
	}{
		{"exact", []string{"api.example.com"}, "api.example.com", true},
		{"exact differs", []string{"api.example.com"}, "api.example.org", false},
		{"case-insensitive host", []string{"api.example.com"}, "API.Example.COM", true},
		{"case-insensitive entry", []string{"API.example.com"}, "api.example.com", true},
		{"trailing dot", []string{"api.example.com"}, "api.example.com.", true},
		{"wildcard one label", []string{"*.example.com"}, "a.example.com", true},
		{"wildcard many labels", []string{"*.example.com"}, "a.b.c.example.com", true},
		{"wildcard excludes the apex", []string{"*.example.com"}, "example.com", false},
		{"wildcard needs a label boundary", []string{"*.example.com"}, "badexample.com", false},
		{"wildcard needs a label", []string{"*.example.com"}, ".example.com", false},
		{"bare star is not a wildcard", []string{"*"}, "example.com", false},
		{"star without dot is not a wildcard", []string{"*example.com"}, "badexample.com", false},
		{"second entry matches", []string{"x.test", "*.example.com"}, "a.example.com", true},
		{"empty host", []string{"*.example.com", ""}, "", false},
		{"empty allowlist", nil, "example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domainAllowed(tt.allow, tt.host); got != tt.want {
				t.Errorf("domainAllowed(%q, %q) = %v, want %v", tt.allow, tt.host, got, tt.want)
			}
		})
	}
}

func TestLEB128(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   []byte
		want uint32
		n    int
	}{
		{[]byte{0x00}, 0, 1},
		{[]byte{0x7f}, 127, 1},
		{[]byte{0x80, 0x01}, 128, 2},
		{[]byte{0xe5, 0x8e, 0x26}, 624485, 3},
		{[]byte{0x80}, 0, 0},
		{nil, 0, 0},
		{[]byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x01}, 0, 0},
	}
	for _, tt := range tests {
		if got, n := leb128(tt.in); got != tt.want || n != tt.n {
			t.Errorf("leb128(%x) = %d, %d; want %d, %d", tt.in, got, n, tt.want, tt.n)
		}
	}
}

func TestHasStartSection(t *testing.T) {
	t.Parallel()
	header := []byte{0, 'a', 's', 'm', 1, 0, 0, 0}
	withStart := append(append([]byte{}, header...), 1, 1, 0, 8, 1, 0)
	if !hasStartSection(withStart) {
		t.Error("start section not found")
	}
	if hasStartSection(append(append([]byte{}, header...), 1, 1, 0)) {
		t.Error("start section found in a module without one")
	}
	if hasStartSection(append(append([]byte{}, header...), 1, 0x80)) {
		t.Error("a truncated length reports a start section")
	}
}
