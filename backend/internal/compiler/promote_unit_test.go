// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// TestPromoteWinnerRefusals names the diagnostics of a promotion that
// cannot happen.
// Verifies: ABT-005.
func TestPromoteWinnerRefusals_ABT_005(t *testing.T) {
	t.Parallel()
	m := experimentsProject(t)
	if _, diags := PromoteWinner(m, "checkoutFlow", "bold"); len(diags) != 1 || diags[0].Code != plxerr.ExperimentKeyUnknown {
		t.Errorf("unknown experiment: %s", list(diags))
	}
	_, diags := PromoteWinner(m, "ctaLabel", "huge")
	if len(diags) != 1 || diags[0].Code != plxerr.WinnerNotAVariant || diags[0].File != expCTA || diags[0].Path != "/variants" {
		t.Errorf("winner that is not a variant: %s", list(diags))
	}
	edit(t, m, expPage, func(d map[string]any) { at(t, d, "root")["bogus"] = 1 })
	if out, diags := PromoteWinner(m, "ctaLabel", "bold"); out != nil || !diags.HasErrors() {
		t.Errorf("a project with errors was promoted: %s", list(diags))
	}
	m = experimentsProject(t)
	edit(t, m, expCTA, func(d map[string]any) {
		d["props"].([]any)[0].(map[string]any)["type"] = "list<string>"
		for _, v := range d["variants"].([]any) {
			v.(map[string]any)["props"] = map[string]any{"label": []any{"a"}}
		}
	})
	if _, diags := PromoteWinner(m, "ctaLabel", "bold"); len(diags) != 1 || diags[0].Code != plxerr.ValueTypeMismatch {
		t.Errorf("prop that cannot be written into a binding: %s", list(diags))
	}
}

// TestPromoteWinnerIsPureAndDeterministic promotes twice and checks that
// the project it read is untouched and the other experiment stays.
// Verifies: ABT-005, CMP-002.
func TestPromoteWinnerIsPureAndDeterministic_ABT_005(t *testing.T) {
	t.Parallel()
	m := experimentsProject(t)
	before := experimentsProject(t)
	a, diags := PromoteWinner(m, "heroLayout", "wide")
	if len(diags) > 0 {
		t.Fatal(list(diags))
	}
	b, _ := PromoteWinner(m, "heroLayout", "wide")
	for name, f := range m {
		if !bytes.Equal(f.Data, before[name].Data) {
			t.Errorf("%s was changed by the promotion", name)
		}
	}
	pageA, errA := fs.ReadFile(a, expPage)
	pageB, errB := fs.ReadFile(b, expPage)
	if errA != nil || errB != nil {
		t.Fatal(errA, errB)
	}
	if !bytes.Equal(pageA, pageB) {
		t.Error("two promotions gave different documents")
	}
	if _, err := fs.Stat(a, expHero); err == nil {
		t.Error("the promoted experiment's document is still in the project")
	}
	if _, err := fs.Stat(a, expCTA); err != nil {
		t.Error("the other experiment's document was dropped")
	}
	if strings.Contains(string(pageA), `"Variant"`) {
		t.Error("the page still has a Variant node")
	}
	if res := Compile(a, DefaultOptions()); len(res.Diagnostics) > 0 {
		t.Errorf("the half-promoted project does not compile:\n%s", list(res.Diagnostics))
	}
}

// TestPromoteInlinesBindings checks the rewriting of expressions: strings
// and other experiments' paths are left alone, and negative constants stay
// atomic.
// Verifies: ABT-005.
func TestPromoteInlinesBindings_ABT_005(t *testing.T) {
	t.Parallel()
	p := &promoter{key: "ctaLabel", winner: "bold", props: map[string]string{"label": `"Buy now"`, "n": "(-2)"}}
	tests := []struct{ in, want string }{
		{`experiment.ctaLabel`, `"bold"`},
		{`experiment.ctaLabel.label`, `"Buy now"`},
		{`experiment.ctaLabel.n * 3`, `(-2) * 3`},
		{`"experiment.ctaLabel" + experiment.ctaLabel`, `"experiment.ctaLabel" + "bold"`},
		{`experiment.heroLayout`, `experiment.heroLayout`},
		{`experiment.ctaLabel.other`, `"bold".other`},
		{`app.experiment.ctaLabel`, `app.experiment.ctaLabel`},
		{`myexperiment.ctaLabel`, `myexperiment.ctaLabel`},
		{`experiment.ctaLabels`, `experiment.ctaLabels`},
		{`'it\'s ' + experiment.ctaLabel`, `'it\'s ' + "bold"`},
	}
	for _, tc := range tests {
		if got := p.inline(tc.in); got != tc.want {
			t.Errorf("inline(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestPxlLiteral checks the constants a prop value is written as.
// Verifies: ABT-005.
func TestPxlLiteral_ABT_005(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typ, raw, want string
		ok             bool
	}{
		{"string", `"a\"b"`, `"a\"b"`, true},
		{"string", `"\u0001"`, ``, false},
		{"bool", `true`, `true`, true},
		{"int", `-4`, `(-4)`, true},
		{"int", `4.5`, ``, false},
		{"double", `2`, `2.0`, true},
		{"double", `-0.5`, `(-0.5)`, true},
		{"string?", `null`, `null`, true},
		{"string", `null`, ``, false},
		{"decimal", `"1.5"`, ``, false},
	}
	for _, tc := range tests {
		got, ok := pxlLiteral(tc.typ, []byte(tc.raw))
		if ok != tc.ok || ok && got != tc.want {
			t.Errorf("pxlLiteral(%s, %s) = %q, %v; want %q, %v", tc.typ, tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}
