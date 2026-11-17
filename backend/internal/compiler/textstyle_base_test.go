// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// pinned sets the app's minRuntimeVersion.
func pinned(t *testing.T, m map[string]any, version string) {
	t.Helper()
	m["minRuntimeVersion"] = version
}

// A style that starts from a typography token takes the token's size and
// weight, and its own colour, for the contrast check (A11Y-003): a base is
// followed as the device follows it.
//
// Verifies: A11Y-003.
func TestTextContrastThroughBase_A11Y_003(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		style string
		want  int
	}{
		{"a base token's size makes large text", `{"base": {"$token": "type.headlineMedium"}}`, 0},
		{"its own size wins over the base's", `{"base": {"$token": "type.headlineMedium"}, "fontSize": 14}`, 1},
		{"its own colour over a token base", `{"base": {"$token": "type.bodyMedium"}, "color": {"$token": "color.primary"}}`, 0},
		{"a base's colour counts when it sets none", `{"base": {"color": {"$token": "color.primary"}}}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := a11yProject(t, "standard", "#777777", "#EEEEEE", "["+text(tc.style, "")+"]")
			edit(t, m, "app.json", func(doc map[string]any) { pinned(t, doc, "0.6.0") })
			res := compileFS(m)
			if got := len(findings(res, plxerr.ContrastInsufficient)); got != tc.want {
				t.Errorf("got %d PLX-1403, want %d:\n%s", got, tc.want, list(res.Diagnostics))
			}
			if got := len(findings(res, plxerr.RuntimeTooOld)); got != 0 {
				t.Errorf("a base at runtime 0.6.0 is no runtime error:\n%s", list(res.Diagnostics))
			}
		})
	}
}

// A style's base arrived in TextStyle revision 2, runtime 0.6.0: an app that
// still supports older runtimes cannot use it (WGT-004, PLX-1119).
//
// Verifies: WGT-004.
func TestTextStyleBaseNeedsItsRuntime_WGT_004(t *testing.T) {
	t.Parallel()
	style := `{"base": {"$token": "type.headlineMedium"}, "color": {"$token": "color.primary"}}`
	m := a11yProject(t, "standard", "#767676", "#EEEEEE", "["+text(style, "")+"]")
	edit(t, m, "app.json", func(doc map[string]any) { pinned(t, doc, "0.5.0") })
	res := compileFS(m)
	got := findings(res, plxerr.RuntimeTooOld)
	if len(got) != 1 || got[0].Path[len(got[0].Path)-len("/props/style/base"):] != "/props/style/base" {
		t.Fatalf("want one PLX-1119 at the style's base:\n%s", list(res.Diagnostics))
	}
}
