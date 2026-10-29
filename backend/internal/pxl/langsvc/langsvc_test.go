// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package langsvc

import (
	"encoding/json"
	"strings"
	"testing"
)

const scope = `{"types":{"Loan":{"fields":{"months":"int"}}},"roots":{"page":"Loan"},"nonNull":[]}`

// Verifies: PXL-007.
func TestHandle(t *testing.T) {
	h := NewHost()
	cases := []struct {
		name, op, scope, src string
		offset               int
		want                 string
	}{
		{name: "check type", op: "check", scope: scope, src: "page.months + 1", want: `"type":"int"`},
		{name: "check error", op: "check", scope: scope, src: "page.nope", want: `"severity":"error"`},
		{name: "complete", op: "complete", scope: scope, src: "page.", offset: 5, want: `"label":"months"`},
		{name: "hover", op: "hover", scope: scope, src: "page.months", offset: 7, want: `"type":"int"`},
		{name: "hover nothing", op: "hover", scope: scope, src: "page.", offset: 2, want: `null`},
		{name: "signature", op: "signature", scope: scope, src: "len(", offset: 4, want: `"label":"len(s string) int"`},
		{name: "signature nothing", op: "signature", scope: scope, src: "page", offset: 2, want: `null`},
		{name: "unknown operation", op: "rename", scope: scope, src: "", want: `"error"`},
		{name: "scope not JSON", op: "check", scope: `{`, src: "1", want: `"error"`},
		{name: "scope with a bad type", op: "check", scope: `{"roots":{"x":"nope"}}`, src: "1", want: `"error"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := h.Handle(c.op, c.scope, c.src, c.offset)
			if !strings.Contains(got, c.want) {
				t.Errorf("got %s, want it to contain %s", got, c.want)
			}
			if !json.Valid([]byte(got)) {
				t.Errorf("not JSON: %s", got)
			}
		})
	}
}

// Verifies: PXL-007.
func TestScopesAreCachedAndBounded(t *testing.T) {
	h := NewHost()
	for i := range cacheSize + 5 {
		s := `{"roots":{"r` + strings.Repeat("x", i) + `":"int"}}`
		if got := h.Handle("check", s, "1", 0); !strings.Contains(got, `"type":"int"`) {
			t.Fatalf("got %s", got)
		}
	}
	if len(h.services) != cacheSize || len(h.order) != cacheSize {
		t.Errorf("cache holds %d scopes, want %d", len(h.services), cacheSize)
	}
}

// Verifies: PXL-007.
func TestNonNullNarrowsTheScope(t *testing.T) {
	h := NewHost()
	open := `{"types":{"Loan":{"fields":{"status":"string?"}}},"roots":{"page":"Loan"}}`
	narrowed := `{"types":{"Loan":{"fields":{"status":"string?"}}},"roots":{"page":"Loan"},"nonNull":["page.status"]}`
	if got := h.Handle("check", open, "page.status", 0); !strings.Contains(got, `"type":"string?"`) && !strings.Contains(got, `"type":""`) {
		t.Errorf("unnarrowed: %s", got)
	}
	if got := h.Handle("check", narrowed, "page.status", 0); !strings.Contains(got, `"type":"string"`) {
		t.Errorf("narrowed: %s", got)
	}
}
