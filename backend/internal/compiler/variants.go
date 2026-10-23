// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// variantType is the reserved type of a Variant node: it shows the subtree
// of the experiment variant assigned to the device (ABT-001, ABT-002).
const variantType = "Variant"

// variantFeature is required by a bundle with a Variant node, so a runtime
// that cannot assign variants refuses the bundle instead of showing the
// control without exposure events (BND-008). It is raised as
// <name>.v1.
const variantFeature = "experiments.variant"

// variantRuntimes lists the runtime each revision of variantFeature first
// shipped in.
var variantRuntimes = []string{"0.6.0"}

// variantInfo is what a Variant node adds to a node: its experiment, and
// the name of the variant each child shows (control first).
type variantInfo struct {
	exp   *experiment
	names []string
}

// buildArms builds the subtrees of a Variant node as its children, the
// control first and the variants by name, so the order is deterministic.
func (u *unit) buildArms(n *node, o owner, all *[]*node) {
	doc := n.doc
	info := &variantInfo{exp: u.experiments[doc.Experiment]}
	n.variant = info
	if doc.Control != nil {
		control := "control"
		if info.exp != nil && info.exp.control != "" {
			control = info.exp.control
		}
		info.names = append(info.names, control)
		n.children = append(n.children, u.buildNode(o, doc.Control, n, "", n.ptr+"/control", all))
	}
	for _, name := range sortedKeys(doc.Variants) {
		arm := doc.Variants[name]
		info.names = append(info.names, name)
		n.children = append(n.children, u.buildNode(o, &arm, n, "", n.ptr+plxerr.Pointer("variants", name), all))
	}
}

// checkVariant checks a Variant node: its experiment, its control and the
// variants it names, that it carries nothing else, and that its subtrees
// fit the node budget (PLX-1702, PLX-2300, PLX-2302).
func (u *unit) checkVariant(n *node) {
	doc, file, ptr := n.doc, n.owner.file(), n.ptr
	e := n.variant.exp
	switch {
	case doc.Experiment == "":
		u.report(plxerr.ExperimentKeyUnknown, file, ptr+"/experiment", "a Variant node names the experiment it belongs to")
	case e == nil:
		u.report(plxerr.ExperimentKeyUnknown, file, ptr+"/experiment", "the project defines no experiment %q", doc.Experiment)
	}
	if doc.Control == nil {
		u.report(plxerr.VariantWithoutControl, file, ptr, "a Variant node needs a control subtree")
	}
	if e != nil {
		for _, name := range sortedKeys(doc.Variants) {
			if name == e.control || !hasName(e.names, name) {
				u.report(plxerr.UnresolvedReference, file, ptr+plxerr.Pointer("variants", name), "experiment %q has no variant %q besides its control", e.doc.Key, name)
			}
		}
	}
	u.variantExtras(n)
	u.requireVariantFeature(vctx{file: file, ptr: ptr + "/type", pl: n.owner.plugin()})
	u.variantBudget(n)
}

// hasName reports whether names contains name.
func hasName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// variantExtras reports what a Variant node must not carry: it is replaced
// by one of its subtrees, so anything else would be lost.
func (u *unit) variantExtras(n *node) {
	doc, file := n.doc, n.owner.file()
	for _, x := range []struct {
		name string
		set  bool
	}{
		{"props", len(doc.Props) > 0},
		{"events", len(doc.Events) > 0},
		{"children", len(doc.Children) > 0},
		{"slots", len(doc.Slots) > 0},
		{"visible", len(doc.Visible) > 0},
		{"semantics", doc.Semantics != nil},
		{"testId", doc.TestID != ""},
		{"responsive", doc.Responsive != nil},
		{"animation", doc.Animation != nil},
		{"direction", doc.Direction != ""},
		{"localeOverrides", len(doc.LocaleOverrides) > 0},
		{"a11yWaivers", len(doc.A11yWaivers) > 0},
	} {
		if x.set {
			u.report(plxerr.UnknownProperty, file, n.ptr+plxerr.Pointer(x.name), "a Variant node holds only its experiment and subtrees, not %s", x.name)
		}
	}
}

// strayVariantFields reports the Variant fields on a node of another type.
func (u *unit) strayVariantFields(n *node) {
	doc, file := n.doc, n.owner.file()
	for _, x := range []struct {
		name string
		set  bool
	}{{"experiment", doc.Experiment != ""}, {"control", doc.Control != nil}, {"variants", len(doc.Variants) > 0}} {
		if x.set {
			u.report(plxerr.UnknownProperty, file, n.ptr+plxerr.Pointer(x.name), "only a Variant node has %s", x.name)
		}
	}
}

// variantBudget reports a Variant node whose subtrees together have more
// nodes than a page may. The registry has no limit of its own for variant
// trees, so the per-page node budget applies (PLX-2302).
func (u *unit) variantBudget(n *node) {
	limit := u.opts.Limits.Get(limits.PageNodes)
	if count := int64(subtreeSize(n)); count > limit {
		u.report(plxerr.VariantTreeExceedsBudget, n.owner.file(), n.ptr, "the subtrees of the Variant node have %d nodes; the limit %s is %d", count, limits.PageNodes, limit)
	}
}

// requireVariantFeature records that a bundle needs the runtime's variant
// support. The feature is listed even when the app's minimum runtime has
// it, because the runtime reads it as a marker.
func (u *unit) requireVariantFeature(c vctx) {
	u.useRevision(variantFeature, variantRuntimes, 1, c)
	if semverLess(u.project.App.Doc.MinRuntimeVersion, variantRuntimes[0]) {
		return
	}
	set := u.features[c.pl]
	if set == nil {
		set = map[string]bool{}
		u.features[c.pl] = set
	}
	set[variantFeature+".v1"] = true
}

// variantTable writes the Variant table of a Variant node: its experiment
// and one arm per child, the control first.
func variantTable(e *valueEnc, n *node) flatbuffers.UOffsetT {
	b := e.b
	arms := make([]flatbuffers.UOffsetT, len(n.children))
	for i, c := range n.children {
		name := e.strs.of(n.variant.names[i])
		fbs.VariantArmStart(b)
		fbs.VariantArmAddName(b, name)
		fbs.VariantArmAddNode(b, c.index)
		arms[i] = fbs.VariantArmEnd(b)
	}
	vec := offsetVector(b, arms)
	fbs.VariantStart(b)
	fbs.VariantAddExperiment(b, e.strs.of(n.doc.Experiment))
	fbs.VariantAddArms(b, vec)
	return fbs.VariantEnd(b)
}
