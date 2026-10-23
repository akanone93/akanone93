// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// experimentsDir is the conformance project of flags, segments,
// experiments, a policy and a Variant node, whose bundles the runtime's
// tests run.
var experimentsDir = filepath.Join("..", "..", "..", "schema", "testdata", "documents", "experiments")

// Documents of the experiments project.
const (
	expPage       = "plugins/shop/pages/home.page.json"
	expVariant    = "root/slots/body/children/0"
	expCTA        = "experiments/ctaLabel.experiment.json"
	expHero       = "experiments/heroLayout.experiment.json"
	expSegment    = "segments/beta-testers.segment.json"
	expFlag       = "flags/newCheckout.flag.json"
	expPolicy     = "policies/secure-release.policy.json"
	expVariantPtr = "/" + expVariant
)

// experimentsProject returns the experiments project in memory.
func experimentsProject(t testing.TB) fstest.MapFS { return project(t, experimentsDir) }

// pageBodyText returns the text node of the page that reads a prop.
func pageText(t testing.TB, doc map[string]any, index string) map[string]any {
	t.Helper()
	return at(t, doc, "root/slots/body/children/"+index+"/props")
}

// TestExperimentsGoldenBundles pins the bundles of the experiments project
// byte for byte and reads the Variant node back: its experiment, its arms
// (control first) and the feature that makes older runtimes refuse it.
// Verifies: ABT-001, ABT-002, ABT-006, BND-008, CMP-002, QA-003.
func TestExperimentsGoldenBundles_ABT_002(t *testing.T) {
	t.Parallel()
	res := Compile(experimentsProject(t), DefaultOptions())
	if len(res.Diagnostics) > 0 {
		t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
	}
	read := readAll(t, res)
	if !slices.Contains(res.Plugins[0].Features, variantFeature+".v1") {
		t.Errorf("plugin features = %v, want the variant feature", res.Plugins[0].Features)
	}
	var arms []string
	for _, s := range read[1].Sections {
		if s.Kind.String() != "page" {
			continue
		}
		pg := fbs.GetRootAsPage(s.Data, 0)
		var n fbs.Node
		var v fbs.Variant
		var arm fbs.VariantArm
		for i := range pg.NodesLength() {
			if !pg.Nodes(&n, i) || n.Variant(&v) == nil {
				continue
			}
			if n.ChildrenLength() != 0 || n.Widget() != 0 {
				t.Errorf("a Variant node has %d children and widget %d", n.ChildrenLength(), n.Widget())
			}
			strs := func(i uint32) string { return string(pg.Strings(int(i))) }
			arms = append(arms, strs(v.Experiment()))
			for j := range v.ArmsLength() {
				v.Arms(&arm, j)
				arms = append(arms, strs(arm.Name()))
			}
		}
	}
	if want := []string{"heroLayout", "control", "wide"}; !slices.Equal(arms, want) {
		t.Errorf("Variant node = %v, want %v", arms, want)
	}
	for _, b := range append([]*Bundle{res.App}, res.Plugins...) {
		checkGolden(t, filepath.Join(goldenRoot, "experiments", b.Key+".pxb"), b.Data)
	}
}

// experimentDefaultsOf returns the control arm the app bundle carries per
// experiment, as "key=control: prop=value ..." in the bundle's order.
func experimentDefaultsOf(t *testing.T, b *bundle.Bundle) []string {
	t.Helper()
	var out []string
	for _, sec := range b.Sections {
		if sec.Kind != bundle.SectionMeta {
			continue
		}
		m := fbs.GetRootAsMeta(sec.Data, 0)
		var e fbs.ExperimentDefaults
		var p fbs.ExperimentProp
		for i := range m.ExperimentsLength() {
			m.Experiments(&e, i)
			line := string(e.Key()) + "=" + string(e.Control()) + ":"
			for j := range e.PropsLength() {
				e.Props(&p, j)
				v := p.Value(nil)
				if v.Kind() != fbs.ValueKindString {
					t.Fatalf("prop %s has value kind %v, want a string", p.Name(), v.Kind())
				}
				line += " " + string(p.Name()) + "=" + stringsOf(t, b)(v.S())
			}
			out = append(out, line)
		}
	}
	return out
}

// TestExperimentDefaultsInTheAppBundle embeds the control arm's value of
// each declared prop in the app bundle, so the runtime shows the control
// when the control document has no entry for the experiment; plugin
// bundles carry none, and an experiment without props adds nothing.
// Verifies: ABT-002, CMP-002.
func TestExperimentDefaultsInTheAppBundle_ABT_002(t *testing.T) {
	t.Parallel()
	res := Compile(experimentsProject(t), DefaultOptions())
	if len(res.Diagnostics) > 0 {
		t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
	}
	read := readAll(t, res)
	if got, want := experimentDefaultsOf(t, read[0]), []string{"ctaLabel=control: label=Buy"}; !slices.Equal(got, want) {
		t.Errorf("app bundle experiments = %v, want %v", got, want)
	}
	for _, b := range read[1:] {
		if got := experimentDefaultsOf(t, b); len(got) != 0 {
			t.Errorf("a plugin bundle carries experiment defaults %v", got)
		}
	}
}

// variantValuesOf returns the value of each prop for each variant the app
// bundle carries, as "key.prop[variant]=value" in the bundle's order.
func variantValuesOf(t *testing.T, b *bundle.Bundle) []string {
	t.Helper()
	var out []string
	for _, sec := range b.Sections {
		if sec.Kind != bundle.SectionMeta {
			continue
		}
		m := fbs.GetRootAsMeta(sec.Data, 0)
		var e fbs.ExperimentDefaults
		var p fbs.ExperimentProp
		var v fbs.ExperimentVariantValue
		for i := range m.ExperimentsLength() {
			m.Experiments(&e, i)
			for j := range e.PropsLength() {
				e.Props(&p, j)
				for k := range p.VariantsLength() {
					p.Variants(&v, k)
					out = append(out, string(e.Key())+"."+string(p.Name())+"["+string(v.Variant())+"]="+stringsOf(t, b)(v.Value(nil).S()))
				}
			}
		}
	}
	return out
}

// TestExperimentDefaultsCarryEveryVariant embeds each variant's value of
// each declared prop, in the order of the experiment document, so a device
// assigned a treatment has the value of its variant.
// Verifies: ABT-002, CMP-002.
func TestExperimentDefaultsCarryEveryVariant_ABT_002(t *testing.T) {
	t.Parallel()
	res := Compile(experimentsProject(t), DefaultOptions())
	if len(res.Diagnostics) > 0 {
		t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
	}
	want := []string{"ctaLabel.label[control]=Buy", "ctaLabel.label[bold]=Buy now", "ctaLabel.label[short]=Go"}
	if got := variantValuesOf(t, readAll(t, res)[0]); !slices.Equal(got, want) {
		t.Errorf("variant values = %v, want %v", got, want)
	}
}

// TestExperimentsAreDeterministic compiles the project twice.
// Verifies: ABT-002, CMP-002.
func TestExperimentsAreDeterministic_ABT_002(t *testing.T) {
	t.Parallel()
	a := Compile(experimentsProject(t), DefaultOptions())
	b := Compile(experimentsProject(t), DefaultOptions())
	if a.Plugins[0].Hash != b.Plugins[0].Hash || a.App.Hash != b.App.Hash {
		t.Error("two compilations of the same project differ")
	}
}

// experimentCase breaks the experiments project in one way.
type experimentCase struct {
	name string
	file string
	// at is the file the diagnostic is reported in, when it is not file.
	at   string
	edit func(t *testing.T, doc map[string]any)
	code plxerr.Code
	ptr  string
	// opts tightens the limits.
	opts func(o *Options)
}

// TestExperimentDiagnostics breaks each document in one way and names the
// diagnostic and its JSON path.
// Verifies: ABT-001, ABT-006, REL-051, GOV-020.
func TestExperimentDiagnostics_ABT_001(t *testing.T) {
	t.Parallel()
	for _, tc := range experimentCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := experimentsProject(t)
			edit(t, m, tc.file, func(doc map[string]any) { tc.edit(t, doc) })
			opts := DefaultOptions()
			if tc.opts != nil {
				tc.opts(&opts)
			}
			res := Compile(m, opts)
			file := tc.file
			if tc.at != "" {
				file = tc.at
			}
			wantDiag(t, res, tc.code, file, tc.ptr)
			if res.App != nil {
				t.Error("a bundle was produced from an invalid project")
			}
		})
	}
}

func experimentCases() []experimentCase {
	return slices.Concat(variantCases(), experimentDocCases(), segmentCases(), flagCases(), approvalPolicyCases(), bindingCases())
}

// variantCases break the Variant node.
func variantCases() []experimentCase {
	variant := func(t *testing.T, doc map[string]any) map[string]any { return at(t, doc, expVariant) }
	return []experimentCase{
		{
			name: "variant without a control", file: expPage, code: plxerr.VariantWithoutControl, ptr: expVariantPtr,
			edit: func(t *testing.T, d map[string]any) { delete(variant(t, d), "control") },
		},
		{
			name: "unknown experiment", file: expPage, code: plxerr.ExperimentKeyUnknown, ptr: expVariantPtr + "/experiment",
			edit: func(t *testing.T, d map[string]any) { variant(t, d)["experiment"] = "checkoutFlow" },
		},
		{
			name: "no experiment", file: expPage, code: plxerr.ExperimentKeyUnknown, ptr: expVariantPtr + "/experiment",
			edit: func(t *testing.T, d map[string]any) { delete(variant(t, d), "experiment") },
		},
		{
			name: "arm of an unknown variant", file: expPage, code: plxerr.UnresolvedReference, ptr: expVariantPtr + "/variants/huge",
			edit: func(t *testing.T, d map[string]any) {
				variant(t, d)["variants"].(map[string]any)["huge"] = map[string]any{"id": "01f0c450-6c00-7000-8000-000000000799", "type": "Text"}
			},
		},
		{
			name: "variant arm named like the control", file: expPage, code: plxerr.UnresolvedReference, ptr: expVariantPtr + "/variants/control",
			edit: func(t *testing.T, d map[string]any) {
				variant(t, d)["variants"].(map[string]any)["control"] = map[string]any{"id": "01f0c450-6c00-7000-8000-000000000798", "type": "Text"}
			},
		},
		{
			name: "variant carries props", file: expPage, code: plxerr.UnknownProperty, ptr: expVariantPtr + "/props",
			edit: func(t *testing.T, d map[string]any) { variant(t, d)["props"] = map[string]any{"data": "x"} },
		},
		{
			name: "experiment key on another node", file: expPage, code: plxerr.UnknownProperty, ptr: "/root/slots/body/experiment",
			edit: func(t *testing.T, d map[string]any) { at(t, d, "root/slots/body")["experiment"] = "heroLayout" },
		},
		{
			name: "variant tree over the node budget", file: expPage, code: plxerr.VariantTreeExceedsBudget, ptr: expVariantPtr,
			edit: func(*testing.T, map[string]any) {},
			opts: func(o *Options) { o.Limits, _ = o.Limits.Tighten(limits.PageNodes, limits.ScopeApp, 1) },
		},
		{
			name: "duplicate node identifier in an arm", file: expPage, code: plxerr.DuplicateID, ptr: expVariantPtr + "/variants/wide/id",
			edit: func(t *testing.T, d map[string]any) {
				variant(t, d)["variants"].(map[string]any)["wide"].(map[string]any)["id"] = at(t, variant(t, d), "control")["id"]
			},
		},
	}
}

// experimentDocCases break the experiment documents.
func experimentDocCases() []experimentCase {
	variants := func(d map[string]any) []any { return d["variants"].([]any) }
	return []experimentCase{
		{
			name: "experiment without a control", file: expCTA, code: plxerr.VariantWithoutControl, ptr: "/variants",
			edit: func(t *testing.T, d map[string]any) { delete(variants(d)[0].(map[string]any), "control") },
		},
		{
			name: "experiment with two controls", file: expCTA, code: plxerr.VariantWithoutControl, ptr: "/variants",
			edit: func(t *testing.T, d map[string]any) { variants(d)[1].(map[string]any)["control"] = true },
		},
		{
			name: "weights over 100", file: expCTA, code: plxerr.VariantWeightsInvalid, ptr: "/variants",
			edit: func(t *testing.T, d map[string]any) { variants(d)[2].(map[string]any)["weight"] = 25 },
		},
		{
			name: "weights under 100", file: expHero, code: plxerr.VariantWeightsInvalid, ptr: "/variants",
			edit: func(t *testing.T, d map[string]any) { variants(d)[1].(map[string]any)["weight"] = 49.5 },
		},
		{
			name: "duplicate variant name", file: expHero, code: plxerr.DuplicateKey, ptr: "/variants/1/name",
			edit: func(t *testing.T, d map[string]any) { variants(d)[1].(map[string]any)["name"] = "control" },
		},
		{
			name: "prop value of the wrong type", file: expCTA, code: plxerr.ValueTypeMismatch, ptr: "/variants/1/props/label",
			edit: func(t *testing.T, d map[string]any) {
				variants(d)[1].(map[string]any)["props"] = map[string]any{"label": 7}
			},
		},
		{
			name: "prop without a value", file: expCTA, code: plxerr.MissingRequiredProp, ptr: "/variants/2/props",
			edit: func(t *testing.T, d map[string]any) { variants(d)[2].(map[string]any)["props"] = map[string]any{} },
		},
		{
			name: "undeclared prop", file: expCTA, code: plxerr.UnknownProp, ptr: "/variants/0/props/colour",
			edit: func(t *testing.T, d map[string]any) {
				variants(d)[0].(map[string]any)["props"].(map[string]any)["colour"] = "red"
			},
		},
		{
			name: "prop of an unknown type", file: expCTA, code: plxerr.UnknownType, ptr: "/props/0/type",
			edit: func(t *testing.T, d map[string]any) { d["props"].([]any)[0].(map[string]any)["type"] = "Colourway" },
		},
		{
			name: "unknown segment", file: expCTA, code: plxerr.UnresolvedReference, ptr: "/segment",
			edit: func(t *testing.T, d map[string]any) { d["segment"] = "everyone" },
		},
		{
			name: "key differs from the file name", file: expHero, code: plxerr.InvalidProjectLayout, ptr: "/key",
			edit: func(t *testing.T, d map[string]any) { d["key"] = "heroWide" },
		},
	}
}

// segmentCases break the segment rule.
func segmentCases() []experimentCase {
	cond := func(t *testing.T, d map[string]any, i string) map[string]any { return at(t, d, "rule/all/"+i) }
	return []experimentCase{
		{
			name: "unknown attribute", file: expSegment, code: plxerr.SegmentRuleInvalid, ptr: "/rule/all/0/attribute",
			edit: func(t *testing.T, d map[string]any) { cond(t, d, "0")["attribute"] = "shoeSize" },
		},
		{
			name: "undeclared user attribute", file: expSegment, code: plxerr.SegmentRuleInvalid, ptr: "/rule/all/1/any/0/attribute",
			edit: func(t *testing.T, d map[string]any) { at(t, d, "rule/all/1/any/0")["attribute"] = "user.plan" },
		},
		{
			name: "ordering a string", file: expSegment, code: plxerr.SegmentRuleInvalid, ptr: "/rule/all/0/operator",
			edit: func(t *testing.T, d map[string]any) { cond(t, d, "0")["operator"] = "gt" },
		},
		{
			name: "in with a single value", file: expSegment, code: plxerr.SegmentRuleInvalid, ptr: "/rule/all/1/any/0/value",
			edit: func(t *testing.T, d map[string]any) { at(t, d, "rule/all/1/any/0")["value"] = "gold" },
		},
		{
			name: "eq with a list", file: expSegment, code: plxerr.SegmentRuleInvalid, ptr: "/rule/all/0/value",
			edit: func(t *testing.T, d map[string]any) { cond(t, d, "0")["value"] = []any{"beta"} },
		},
		{
			name: "empty list", file: expSegment, code: plxerr.SegmentRuleInvalid, ptr: "/rule/all/1/any/0/value",
			edit: func(t *testing.T, d map[string]any) { at(t, d, "rule/all/1/any/0")["value"] = []any{} },
		},
		{
			name: "string where a build number belongs", file: expSegment, code: plxerr.SegmentRuleInvalid, ptr: "/rule/all/1/any/1/value",
			edit: func(t *testing.T, d map[string]any) { at(t, d, "rule/all/1/any/1")["value"] = "100" },
		},
		{
			name: "member of a list of the wrong type", file: expSegment, code: plxerr.SegmentRuleInvalid, ptr: "/rule/all/1/any/0/value/1",
			edit: func(t *testing.T, d map[string]any) { at(t, d, "rule/all/1/any/0")["value"] = []any{"gold", 3} },
		},
	}
}

// flagCases break the flag document.
func flagCases() []experimentCase {
	return []experimentCase{
		{
			name: "unknown flag type", file: expFlag, code: plxerr.UnknownFlagType, ptr: "/type",
			edit: func(t *testing.T, d map[string]any) { d["type"] = "enum" },
		},
		{
			name: "default of the wrong type", file: expFlag, code: plxerr.ValueTypeMismatch, ptr: "/default",
			edit: func(t *testing.T, d map[string]any) { d["default"] = "no" },
		},
		{
			name: "segment value of the wrong type", file: expFlag, code: plxerr.ValueTypeMismatch, ptr: "/segments/0/value",
			edit: func(t *testing.T, d map[string]any) { d["segments"].([]any)[0].(map[string]any)["value"] = 1 },
		},
		{
			name: "unknown segment", file: expFlag, code: plxerr.UnresolvedReference, ptr: "/segments/0/segment",
			edit: func(t *testing.T, d map[string]any) { d["segments"].([]any)[0].(map[string]any)["segment"] = "nobody" },
		},
		{
			name: "schema on a bool flag", file: expFlag, code: plxerr.UnknownProperty, ptr: "/schema",
			edit: func(t *testing.T, d map[string]any) { d["schema"] = map[string]any{"type": "boolean"} },
		},
		{
			name: "flag named like an app flag", file: "app.json", at: expFlag, code: plxerr.DuplicateKey, ptr: "/key",
			edit: func(t *testing.T, d map[string]any) {
				d["flags"] = []any{map[string]any{"name": "newCheckout", "type": "bool", "default": true}}
			},
		},
	}
}

// approvalPolicyCases break the policy document.
func approvalPolicyCases() []experimentCase {
	return []experimentCase{
		{
			name: "unknown environment", file: expPolicy, code: plxerr.PolicyDocumentInvalid, ptr: "/match/environments/0",
			edit: func(t *testing.T, d map[string]any) { at(t, d, "match")["environments"] = []any{"moon"} },
		},
		{
			name: "risk on resources it is not computed for", file: expPolicy, code: plxerr.PolicyDocumentInvalid, ptr: "/match/risk",
			edit: func(t *testing.T, d map[string]any) { at(t, d, "match")["resources"] = []any{"limit"} },
		},
		{
			name: "more approvals than named approvers", file: expPolicy, code: plxerr.PolicyDocumentInvalid, ptr: "/require/approvers",
			edit: func(t *testing.T, d map[string]any) {
				r := at(t, d, "require")
				delete(r, "roles")
				r["users"] = []any{"ada"}
			},
		},
		{
			name: "distinct teams with one approver", file: expPolicy, code: plxerr.PolicyDocumentInvalid, ptr: "/require/distinctTeams",
			edit: func(t *testing.T, d map[string]any) { at(t, d, "require")["approvers"] = 1 },
		},
		{
			name: "sequential stages without approvers to order", file: expPolicy, code: plxerr.PolicyDocumentInvalid, ptr: "/require/stages",
			edit: func(t *testing.T, d map[string]any) {
				r := at(t, d, "require")
				delete(r, "roles")
				r["stages"] = "sequential"
			},
		},
	}
}

// bindingCases break the PXL bindings of experiments.
func bindingCases() []experimentCase {
	const label = "/root/slots/body/children/1/props/data"
	bind := func(expr string) func(t *testing.T, d map[string]any) {
		return func(t *testing.T, d map[string]any) {
			pageText(t, d, "1")["data"] = map[string]any{"$expr": expr}
		}
	}
	return []experimentCase{
		{name: "prop used as a number", file: expPage, code: plxerr.PXLTypeMismatch, ptr: label, edit: bind("experiment.ctaLabel.label + 1")},
		{name: "undeclared prop in a binding", file: expPage, code: plxerr.PXLTypeMismatch, ptr: label, edit: bind("experiment.ctaLabel.colour")},
		{name: "unknown experiment", file: expPage, code: plxerr.PXLUnknownField, ptr: label, edit: bind("experiment.price")},
		{
			name: "variant name that does not exist", file: expPage, code: plxerr.PXLUnknownEnumMember, ptr: label,
			edit: bind(`experiment.ctaLabel == "huge" ? "a" : "b"`),
		},
		{name: "prop of an experiment without props", file: expPage, code: plxerr.PXLTypeMismatch, ptr: label, edit: bind("experiment.heroLayout.label")},
	}
}

// TestVariantFeatureIsRequired checks that a Variant node raises the
// feature: refused when the app's minimum runtime is older and does not
// allow raising, raised with a warning when it does.
// Verifies: ABT-002, BND-008, WGT-004.
func TestVariantFeatureIsRequired_BND_008(t *testing.T) {
	t.Parallel()
	m := experimentsProject(t)
	edit(t, m, "app.json", func(d map[string]any) { d["minRuntimeVersion"] = "0.5.0" })
	res := Compile(m, DefaultOptions())
	wantDiag(t, res, plxerr.RuntimeTooOld, expPage, expVariantPtr+"/type")

	edit(t, m, "app.json", func(d map[string]any) { d["requiredFeatures"] = "raise" })
	res = Compile(m, DefaultOptions())
	wantDiag(t, res, plxerr.RequiredFeaturesRaised, expPage, expVariantPtr+"/type")
	if res.Plugins == nil || !slices.Contains(res.Plugins[0].Features, variantFeature+".v1") {
		t.Errorf("features = %v", res.Plugins)
	}
	if !strings.Contains(list(res.Diagnostics), variantFeature) {
		t.Errorf("the warning does not name the feature:\n%s", list(res.Diagnostics))
	}
}
