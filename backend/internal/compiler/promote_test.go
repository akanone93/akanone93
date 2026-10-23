// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// promoteAll promotes both experiments of the project.
func promoteAll(t *testing.T, m fs.FS, ctaWinner, heroWinner string) fs.FS {
	t.Helper()
	step, diags := PromoteWinner(m, "ctaLabel", ctaWinner)
	if len(diags) > 0 {
		t.Fatalf("promote ctaLabel:\n%s", list(diags))
	}
	out, diags := PromoteWinner(step, "heroLayout", heroWinner)
	if len(diags) > 0 {
		t.Fatalf("promote heroLayout:\n%s", list(diags))
	}
	return out
}

// written returns the experiments project written with the winners
// directly: the control and "bold" copy, no experiment documents.
func written(t *testing.T, heroText, label, branch string) fstest.MapFS {
	t.Helper()
	m := experimentsProject(t)
	delete(m, expCTA)
	delete(m, expHero)
	edit(t, m, expPage, func(d map[string]any) {
		body := at(t, d, "root/slots/body")
		body["children"] = []any{
			map[string]any{"id": "01f0c450-6c00-7000-8000-000000000799", "type": "Text", "props": map[string]any{"data": heroText}},
			map[string]any{"id": "01f0c450-6c00-7000-8000-000000000710", "type": "Text", "props": map[string]any{"data": map[string]any{"$expr": label}}},
			map[string]any{"id": "01f0c450-6c00-7000-8000-000000000711", "type": "Text", "props": map[string]any{"data": map[string]any{"$expr": branch}}},
		}
	})
	return m
}

// TestPromoteWinnerIsByteIdentical promotes the winners of both experiments
// and compiles the result against a project written with them: the bundles
// must be the same bytes.
// Verifies: ABT-005, CMP-002.
func TestPromoteWinnerIsByteIdentical_ABT_005(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, cta, hero string
		want            fstest.MapFS
	}{
		{"winners are variants", "bold", "wide", nil},
		{"winners are controls", "control", "control", nil},
	}
	cases[0].want = written(t, "Welcome back, we missed you", `"Buy now"`, `"bold" == "bold" ? "bold" : "other"`)
	cases[1].want = written(t, "Welcome", `"Buy"`, `"control" == "bold" ? "bold" : "other"`)
	edit(t, cases[0].want, expPage, func(d map[string]any) {
		at(t, d, "root/slots/body/children/0")["id"] = "01f0c450-6c00-7000-8000-00000000070f"
	})
	edit(t, cases[1].want, expPage, func(d map[string]any) {
		at(t, d, "root/slots/body/children/0")["id"] = "01f0c450-6c00-7000-8000-00000000070e"
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			promoted := Compile(promoteAll(t, experimentsProject(t), tc.cta, tc.hero), DefaultOptions())
			direct := Compile(tc.want, DefaultOptions())
			if len(promoted.Diagnostics) > 0 || len(direct.Diagnostics) > 0 {
				t.Fatalf("promoted:\n%sdirect:\n%s", list(promoted.Diagnostics), list(direct.Diagnostics))
			}
			if !bytes.Equal(promoted.Plugins[0].Data, direct.Plugins[0].Data) || !bytes.Equal(promoted.App.Data, direct.App.Data) {
				t.Error("the promoted project does not compile to the bundles of the project written with the winner")
			}
			for _, f := range promoted.Plugins[0].Features {
				if strings.HasPrefix(f, variantFeature) {
					t.Errorf("the promoted bundle still requires %s", f)
				}
			}
		})
	}
}
