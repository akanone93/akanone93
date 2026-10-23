// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"math"
	"slices"
	"strconv"
	"strings"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
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
	// control is the checked value of the prop for the control variant;
	// nil when it did not check, or when the page is checked alone.
	control *value
	// variants are the checked values of the prop for every variant that
	// gave one that checked, in the order of the experiment document.
	variants []variantValue
}

// variantValue is the checked value of a prop for one variant.
type variantValue struct {
	variant string
	value   *value
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
	declared := u.declareExperimentProps(e)
	for i, v := range e.doc.Variants {
		vp := plxerr.Pointer("variants", strconv.Itoa(i), "props")
		for _, name := range sortedKeys(v.Props) {
			if !declared[name] {
				u.reportDoc(plxerr.UnknownProp, e.file, vp+plxerr.Pointer(name), "the experiment declares no prop %q", name)
			}
		}
		for j := range e.props {
			u.checkVariantProp(e, &e.props[j], v, vp)
		}
	}
}

// declareExperimentProps claims the names of the declared props, keeps those
// whose type checks on the experiment, sorted by name, and returns the set of
// declared names.
func (u *unit) declareExperimentProps(e *experiment) map[string]bool {
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
	return declared
}

// checkVariantProp checks the value one variant gives one prop and records it
// on the prop; the control variant's value becomes the prop's default.
func (u *unit) checkVariantProp(e *experiment, p *experimentProp, v schema.ExperimentVariant, vp string) {
	raw, ok := v.Props[p.name]
	if !ok {
		u.reportDoc(plxerr.MissingRequiredProp, e.file, vp, "variant %q gives no value for the prop %q", v.Name, p.name)
		return
	}
	if u.validating {
		return
	}
	checked := u.checkRaw(vctx{file: e.file, ptr: vp + plxerr.Pointer(p.name), code: plxerr.ValueTypeMismatch}, raw, p.te)
	if v.Name == e.control {
		p.control = checked
	}
	if checked != nil {
		p.variants = append(p.variants, variantValue{variant: v.Name, value: checked})
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

// experimentDefaults writes the control arm of each experiment that
// declares props, sorted by key, for the runtime to default to when the
// control document has no entry for the experiment (ABT-002). A prop whose
// control value did not check is left out; the diagnostic is already
// reported. Each prop carries its value for every variant as well. It is 0 when no experiment declares props.
func (u *unit) experimentDefaults(e *valueEnc) flatbuffers.UOffsetT {
	b := e.b
	var offs []flatbuffers.UOffsetT
	for _, key := range sortedKeys(u.experiments) {
		exp := u.experiments[key]
		if len(exp.props) == 0 || exp.control == "" {
			continue
		}
		props := make([]flatbuffers.UOffsetT, 0, len(exp.props))
		for _, p := range exp.props {
			val := e.value(p.control)
			if val == 0 {
				continue
			}
			name, variants := b.CreateString(p.name), variantValues(e, p.variants)
			fbs.ExperimentPropStart(b)
			fbs.ExperimentPropAddName(b, name)
			fbs.ExperimentPropAddValue(b, val)
			fbs.ExperimentPropAddVariants(b, variants)
			props = append(props, fbs.ExperimentPropEnd(b))
		}
		pv, k, c := offsetVector(b, props), b.CreateString(key), b.CreateString(exp.control)
		fbs.ExperimentDefaultsStart(b)
		fbs.ExperimentDefaultsAddKey(b, k)
		fbs.ExperimentDefaultsAddControl(b, c)
		fbs.ExperimentDefaultsAddProps(b, pv)
		offs = append(offs, fbs.ExperimentDefaultsEnd(b))
	}
	if len(offs) == 0 {
		return 0
	}
	return offsetVector(b, offs)
}

// variantValues writes the value of a prop for each variant, in the order
// of the experiment document, so that a device assigned a treatment reads
// its own variant's value (ABT-002). It is 0 when no variant has one.
func variantValues(e *valueEnc, values []variantValue) flatbuffers.UOffsetT {
	b := e.b
	offs := make([]flatbuffers.UOffsetT, 0, len(values))
	for _, v := range values {
		val := e.value(v.value)
		if val == 0 {
			continue
		}
		name := b.CreateString(v.variant)
		fbs.ExperimentVariantValueStart(b)
		fbs.ExperimentVariantValueAddVariant(b, name)
		fbs.ExperimentVariantValueAddValue(b, val)
		offs = append(offs, fbs.ExperimentVariantValueEnd(b))
	}
	if len(offs) == 0 {
		return 0
	}
	return offsetVector(b, offs)
}
