// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// experimentsDir is the conformance project of flags, segments,
// experiments, a policy and a Variant node.
var experimentsDir = filepath.Join("..", "..", "..", "schema", "testdata", "documents", "experiments")

// experimentsFS returns an in-memory copy of the experiments project.
func experimentsFS(t *testing.T) fstest.MapFS {
	t.Helper()
	m := fstest.MapFS{}
	err := filepath.WalkDir(experimentsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		rel, _ := filepath.Rel(experimentsDir, path)
		m[filepath.ToSlash(rel)] = &fstest.MapFile{Data: data}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestLoadsTheRolloutDocuments loads the project and finds each document.
// Verifies: ABT-001, ABT-006, REL-051, GOV-020, SCH-006.
func TestLoadsTheRolloutDocuments_ABT_001(t *testing.T) {
	t.Parallel()
	p, diags := newLoader(t).Load(experimentsFS(t))
	if len(diags) > 0 {
		t.Fatalf("diagnostics:\n%s", list(diags))
	}
	if len(p.Segments) != 1 || len(p.Flags) != 1 || len(p.Experiments) != 2 || len(p.Policies) != 1 {
		t.Fatalf("segments %d, flags %d, experiments %d, policies %d", len(p.Segments), len(p.Flags), len(p.Experiments), len(p.Policies))
	}
	root := p.Plugins[0].Pages[0].Doc.Root.Slots["body"].One.Children[0]
	if root.Type != "Variant" || root.Experiment != "heroLayout" || root.Control == nil || root.Variants["wide"].ID == "" {
		t.Errorf("Variant node = %+v", root)
	}
}

// TestRolloutDocumentsAreValidatedStructurally rejects each document in
// one way and names the code and the path.
// Verifies: ABT-001, ABT-006, REL-051, GOV-020, SCH-040.
func TestRolloutDocumentsAreValidatedStructurally_ABT_001(t *testing.T) {
	t.Parallel()
	const (
		page, seg, flag = "plugins/shop/pages/home.page.json", "segments/beta-testers.segment.json", "flags/newCheckout.flag.json"
		exp, pol        = "experiments/ctaLabel.experiment.json", "policies/secure-release.policy.json"
	)
	tests := []struct {
		name string
		file string
		edit func(t *testing.T, d map[string]any)
		code plxerr.Code
		path string
	}{
		{"Variant control that is not a node", page, func(t *testing.T, d map[string]any) {
			at(t, d, "root/slots/body/children/0")["control"] = "Welcome"
		}, plxerr.WrongJSONType, "/root/slots/body/children/0/control"},
		{"Variant arm named with capitals", page, func(t *testing.T, d map[string]any) {
			at(t, d, "root/slots/body/children/0/variants")["Wide"] = map[string]any{"id": "01f0c450-6c00-7000-8000-000000000799", "type": "Text"}
		}, plxerr.InvalidFormat, "/root/slots/body/children/0/variants/Wide"},
		{"Variant experiment that is not a name", page, func(t *testing.T, d map[string]any) {
			at(t, d, "root/slots/body/children/0")["experiment"] = "hero-layout"
		}, plxerr.InvalidFormat, "/root/slots/body/children/0/experiment"},
		{"segment rule without a form", seg, func(t *testing.T, d map[string]any) { d["rule"] = map[string]any{} }, plxerr.InvalidStructure, "/rule"},
		{"segment rule with a condition and a group", seg, func(t *testing.T, d map[string]any) {
			at(t, d, "rule/all/0")["any"] = []any{}
		}, plxerr.InvalidStructure, "/rule/all/0"},
		{"segment operator", seg, func(t *testing.T, d map[string]any) { at(t, d, "rule/all/0")["operator"] = "like" }, plxerr.InvalidEnumValue, "/rule/all/0/operator"},
		{"segment group that is empty", seg, func(t *testing.T, d map[string]any) { d["rule"] = map[string]any{"all": []any{}} }, plxerr.OutOfRange, "/rule/all"},
		{"segment without a name", seg, func(t *testing.T, d map[string]any) { delete(d, "name") }, plxerr.MissingProperty, ""},
		{"flag without a default", flag, func(t *testing.T, d map[string]any) { delete(d, "default") }, plxerr.MissingProperty, ""},
		{"flag key that is not a name", flag, func(t *testing.T, d map[string]any) { d["key"] = "new-checkout" }, plxerr.InvalidFormat, "/key"},
		{"flag segment value without a segment", flag, func(t *testing.T, d map[string]any) {
			delete(at(t, d, "segments/0"), "segment")
		}, plxerr.MissingProperty, "/segments/0"},
		{"experiment scope", exp, func(t *testing.T, d map[string]any) { d["scope"] = "galaxy" }, plxerr.InvalidEnumValue, "/scope"},
		{"experiment with one variant", exp, func(t *testing.T, d map[string]any) {
			d["variants"] = d["variants"].([]any)[:1]
		}, plxerr.OutOfRange, "/variants"},
		{"variant weight above 100", exp, func(t *testing.T, d map[string]any) {
			at(t, d, "variants/0")["weight"] = 150
		}, plxerr.OutOfRange, "/variants/0/weight"},
		{"experiment without a primary metric", exp, func(t *testing.T, d map[string]any) { delete(d, "primaryMetric") }, plxerr.MissingProperty, ""},
		{"holdout above 100", exp, func(t *testing.T, d map[string]any) { d["holdout"] = 101 }, plxerr.OutOfRange, "/holdout"},
		{"minimum sample size of zero", exp, func(t *testing.T, d map[string]any) { d["minimumSampleSize"] = 0 }, plxerr.OutOfRange, "/minimumSampleSize"},
		{"metric kind", exp, func(t *testing.T, d map[string]any) { at(t, d, "primaryMetric")["kind"] = "median" }, plxerr.InvalidEnumValue, "/primaryMetric/kind"},
		{"policy without require", pol, func(t *testing.T, d map[string]any) { delete(d, "require") }, plxerr.MissingProperty, ""},
		{"policy approvers of zero", pol, func(t *testing.T, d map[string]any) { at(t, d, "require")["approvers"] = 0 }, plxerr.OutOfRange, "/require/approvers"},
		{"policy resource", pol, func(t *testing.T, d map[string]any) {
			at(t, d, "match")["resources"] = []any{"moon"}
		}, plxerr.InvalidEnumValue, "/match/resources/0"},
		{"policy stages", pol, func(t *testing.T, d map[string]any) { at(t, d, "require")["stages"] = "chaotic" }, plxerr.InvalidEnumValue, "/require/stages"},
		{"policy unknown property", pol, func(t *testing.T, d map[string]any) { at(t, d, "require")["quorum"] = 2 }, plxerr.UnknownProperty, "/require/quorum"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fsys := experimentsFS(t)
			edit(t, fsys, tt.file, func(doc map[string]any) { tt.edit(t, doc) })
			_, diags := newLoader(t).Load(fsys)
			wantDiagnostic(t, diags, tt.code, tt.file, tt.path)
		})
	}
}

// TestRolloutDocumentsHaveTheirPlace checks the layout of the new
// documents.
// Verifies: SCH-006, ABT-001.
func TestRolloutDocumentsHaveTheirPlace_ABT_001(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]Place{
		"segments/beta-testers.segment.json":   {Kind: KindSegment, Key: "beta-testers"},
		"policies/secure-release.policy.json":  {Kind: KindPolicy, Key: "secure-release"},
		"flags/newCheckout.flag.json":          {Kind: KindFlag, Key: "newCheckout"},
		"experiments/ctaLabel.experiment.json": {Kind: KindExperiment, Key: "ctaLabel"},
	} {
		if got, ok := PlaceOf(path); !ok || got != want {
			t.Errorf("PlaceOf(%q) = %+v, %v; want %+v", path, got, ok, want)
		}
	}
	for _, path := range []string{
		"flags/new-checkout.flag.json", "experiments/CtaLabel.experiment.json", "segments/Beta.segment.json",
		"segments/beta.flag.json", "flags/a/b.flag.json", "experiments/.experiment.json",
	} {
		if _, ok := PlaceOf(path); ok {
			t.Errorf("PlaceOf(%q) accepted a path outside the layout", path)
		}
	}
}
