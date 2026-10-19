// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/pxl"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// experiment is an experiment of the project as the compiler checks it.
type experiment struct {
	doc  *schema.ExperimentDocument
	file string
	// control is the name of the control variant; names are all variant
	// names in document order.
	control string
	names   []string
	// props are the declared props that checked, sorted by name.
	props []experimentProp
}

// experimentProp is a prop an experiment varies and its type.
type experimentProp struct {
	name string
	te   *texpr
}

// weightTolerance absorbs the rounding of decimal weights summed in
// floating point.
const weightTolerance = 1e-6

// indexExperiments indexes the experiments and checks their variants and
// props (PLX-1702, PLX-1703).
func (u *unit) indexExperiments() {
	keys := u.newKeys("experiment")
	for _, l := range u.project.Experiments {
		if l.Doc == nil {
			continue
		}
		e := &experiment{doc: l.Doc, file: l.Source.File}
		u.claimID(l.Doc.ID, e.file, "/id")
		u.docKey(e.file, ".experiment.json", l.Doc.Key)
		keys.claim(l.Doc.Key, e.file, "/key")
		u.checkVariantSet(e)
		u.checkExperimentProps(e)
		if seg := l.Doc.Segment; seg != "" && !u.segments[seg] {
			u.reportDoc(plxerr.UnresolvedReference, e.file, "/segment", "no segment %q", seg)
		}
		u.experiments[l.Doc.Key] = e
	}
}

// checkVariantSet checks that exactly one variant is the control and that
// the weights sum to 100.
func (u *unit) checkVariantSet(e *experiment) {
	names := u.newKeys("variant")
	var sum float64
	controls := 0
	for i, v := range e.doc.Variants {
		names.claim(v.Name, e.file, plxerr.Pointer("variants", strconv.Itoa(i), "name"))
		e.names = append(e.names, v.Name)
		sum += v.Weight
		if v.Control != nil && *v.Control {
			controls++
			e.control = v.Name
		}
	}
	if controls != 1 {
		u.reportDoc(plxerr.VariantWithoutControl, e.file, "/variants", "%d of the variants are marked as the control; exactly one must be", controls)
	}
	if math.Abs(sum-100) > weightTolerance {
		u.reportDoc(plxerr.VariantWeightsInvalid, e.file, "/variants", "the weights sum to %s, not 100", strconv.FormatFloat(sum, 'f', -1, 64))
	}
}

// checkExperimentProps checks the declared props and the value each
// variant gives them.
func (u *unit) checkExperimentProps(e *experiment) {
	names := u.newKeys("prop")
	declared := map[string]bool{}
	for i, p := range e.doc.Props {
		ptr := plxerr.Pointer("props", strconv.Itoa(i))
		names.claim(p.Name, e.file, ptr+"/name")
		declared[p.Name] = true
		if te := u.checkType(nil, p.Type, e.file, ptr+"/type"); te != nil {
			e.props = append(e.props, experimentProp{name: p.Name, te: te})
		}
	}
	slices.SortFunc(e.props, func(a, b experimentProp) int { return strings.Compare(a.name, b.name) })
	for i, v := range e.doc.Variants {
		vp := plxerr.Pointer("variants", strconv.Itoa(i), "props")
		for _, name := range sortedKeys(v.Props) {
			if !declared[name] {
				u.reportDoc(plxerr.UnknownProp, e.file, vp+plxerr.Pointer(name), "the experiment declares no prop %q", name)
			}
		}
		for _, p := range e.props {
			raw, ok := v.Props[p.name]
			if !ok {
				u.reportDoc(plxerr.MissingRequiredProp, e.file, vp, "variant %q gives no value for the prop %q", v.Name, p.name)
				continue
			}
			if u.focus == "" {
				u.checkRaw(vctx{file: e.file, ptr: vp + plxerr.Pointer(p.name), code: plxerr.ValueTypeMismatch}, raw, p.te)
			}
		}
	}
}

// variantTypeName is the PXL enum of an experiment's variant names.
func variantTypeName(key string) string { return "PluxVariant" + upperFirst(key) }

// experimentTypes returns the PXL types of the experiment root: the root
// object has a field per experiment, an enum of its variants, and a field
// "<key>.<prop>" per declared prop, which the checker reads as
// experiment.<key>.<prop> (ABT-001).
func (u *unit) experimentTypes() map[string]pxl.TypeSpec {
	types := map[string]pxl.TypeSpec{}
	var fields [][2]string
	for _, key := range sortedKeys(u.experiments) {
		e := u.experiments[key]
		if len(e.names) == 0 {
			continue
		}
		types[variantTypeName(key)] = pxl.TypeSpec{Enum: slices.Clone(e.names)}
		fields = append(fields, [2]string{key, variantTypeName(key)})
		for _, p := range e.props {
			fields = append(fields, [2]string{key + "." + p.name, p.te.String()})
		}
	}
	types["PluxExperiments"] = objectType(fields)
	return types
}
