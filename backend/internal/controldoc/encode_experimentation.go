// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package controldoc

import (
	"cmp"
	"slices"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/nightCode42/plux3/backend/internal/controldoc/controlfbs"
)

// wireOf returns the wire value of the member of a list: the members of the
// lists in this package are in the order of their wire enumerations, which
// start at one (zero is Unspecified).
func wireOf[T comparable](list []T, member T) byte {
	return byte(slices.Index(list, member) + 1) //nolint:gosec // the lists are short
}

// memberOf is the inverse of wireOf; ok is false for Unspecified and for a
// value past the end of the list.
func memberOf[T any](list []T, wire byte) (member T, ok bool) {
	if wire == 0 || int(wire) > len(list) {
		return member, false
	}
	return list[wire-1], true
}

// experimentation holds the offsets of the three parts a document adds to
// the base tables.
type experimentation struct {
	flags, experiments, holdout flatbuffers.UOffsetT
}

// writeExperimentation writes the flags, the experiments and the holdout of
// a document, flags and experiments sorted by key.
func writeExperimentation(b *flatbuffers.Builder, d Document) experimentation {
	var out experimentation
	flags := slices.Clone(d.Flags)
	slices.SortFunc(flags, func(x, y FlagValue) int { return cmp.Compare(x.Key, y.Key) })
	tables := make([]flatbuffers.UOffsetT, len(flags))
	for i, f := range flags {
		tables[i] = writeFlag(b, f)
	}
	out.flags = writeTables(b, tables, controlfbs.ControlDocumentStartFlagsVector)

	exps := slices.Clone(d.Experiments)
	slices.SortFunc(exps, func(x, y Allocation) int { return cmp.Compare(x.Key, y.Key) })
	tables = make([]flatbuffers.UOffsetT, len(exps))
	for i, a := range exps {
		tables[i] = writeAllocation(b, a)
	}
	out.experiments = writeTables(b, tables, controlfbs.ControlDocumentStartExperimentsVector)

	if h := d.Holdout; h != nil {
		salt := b.CreateString(h.Salt)
		controlfbs.GlobalHoldoutStart(b)
		controlfbs.GlobalHoldoutAddBasisPoints(b, h.BasisPoints)
		controlfbs.GlobalHoldoutAddSalt(b, salt)
		out.holdout = controlfbs.GlobalHoldoutEnd(b)
	}
	return out
}

// writeTables writes a vector of already built tables, in order, with the
// start function the generated code names for the vector.
func writeTables(b *flatbuffers.Builder, tables []flatbuffers.UOffsetT, start func(*flatbuffers.Builder, int) flatbuffers.UOffsetT) flatbuffers.UOffsetT {
	start(b, len(tables))
	for i := len(tables) - 1; i >= 0; i-- {
		b.PrependUOffsetT(tables[i])
	}
	return b.EndVector(len(tables))
}

// writeStrings writes a vector of strings, in order.
func writeStrings(b *flatbuffers.Builder, values []string, start func(*flatbuffers.Builder, int) flatbuffers.UOffsetT) flatbuffers.UOffsetT {
	offsets := make([]flatbuffers.UOffsetT, len(values))
	for i, v := range values {
		offsets[i] = b.CreateString(v)
	}
	return writeTables(b, offsets, start)
}

func writeFlag(b *flatbuffers.Builder, f FlagValue) flatbuffers.UOffsetT {
	key, value := b.CreateString(f.Key), b.CreateString(f.Default)
	segments := make([]flatbuffers.UOffsetT, len(f.Segments))
	for i, s := range f.Segments {
		segments[i] = writeFlagSegment(b, s)
	}
	vector := writeTables(b, segments, controlfbs.FlagValueStartSegmentsVector)
	controlfbs.FlagValueStart(b)
	controlfbs.FlagValueAddKey(b, key)
	controlfbs.FlagValueAddType(b, controlfbs.FlagType(wireOf(FlagTypes(), f.Type)))
	controlfbs.FlagValueAddDefaultValue(b, value)
	controlfbs.FlagValueAddSegments(b, vector)
	return controlfbs.FlagValueEnd(b)
}

func writeFlagSegment(b *flatbuffers.Builder, s FlagSegmentValue) flatbuffers.UOffsetT {
	segment, value := b.CreateString(s.Segment), b.CreateString(s.Value)
	rule := writeRule(b, s.Rule)
	controlfbs.FlagSegmentValueStart(b)
	controlfbs.FlagSegmentValueAddSegment(b, segment)
	controlfbs.FlagSegmentValueAddRule(b, rule)
	controlfbs.FlagSegmentValueAddValue(b, value)
	return controlfbs.FlagSegmentValueEnd(b)
}

// writeRule writes a targeting rule and, first, the rules inside it.
func writeRule(b *flatbuffers.Builder, r Rule) flatbuffers.UOffsetT {
	var attribute, values, all, anyOf flatbuffers.UOffsetT
	if r.Attribute != "" {
		attribute = b.CreateString(r.Attribute)
	}
	if len(r.Values) > 0 {
		values = writeStrings(b, r.Values, controlfbs.TargetingRuleStartValuesVector)
	}
	if len(r.All) > 0 {
		all = writeRules(b, r.All, controlfbs.TargetingRuleStartAllVector)
	}
	if len(r.Any) > 0 {
		anyOf = writeRules(b, r.Any, controlfbs.TargetingRuleStartAnyVector)
	}
	controlfbs.TargetingRuleStart(b)
	if attribute != 0 {
		controlfbs.TargetingRuleAddAttribute(b, attribute)
		controlfbs.TargetingRuleAddOperator(b, controlfbs.RuleOperator(wireOf(ruleOperators, r.Operator)))
		controlfbs.TargetingRuleAddValueType(b, controlfbs.RuleValueType(wireOf(valueTypes, r.ValueType)))
		controlfbs.TargetingRuleAddValues(b, values)
	}
	if all != 0 {
		controlfbs.TargetingRuleAddAll(b, all)
	}
	if anyOf != 0 {
		controlfbs.TargetingRuleAddAny(b, anyOf)
	}
	return controlfbs.TargetingRuleEnd(b)
}

func writeRules(b *flatbuffers.Builder, rules []Rule, start func(*flatbuffers.Builder, int) flatbuffers.UOffsetT) flatbuffers.UOffsetT {
	tables := make([]flatbuffers.UOffsetT, len(rules))
	for i, r := range rules {
		tables[i] = writeRule(b, r)
	}
	return writeTables(b, tables, start)
}

func writeAllocation(b *flatbuffers.Builder, a Allocation) flatbuffers.UOffsetT {
	key, salt := b.CreateString(a.Key), b.CreateString(a.Salt)
	var layer, segment flatbuffers.UOffsetT
	if a.Layer != "" {
		layer = b.CreateString(a.Layer)
	}
	variants := make([]flatbuffers.UOffsetT, len(a.Variants))
	for i, v := range a.Variants {
		name := b.CreateString(v.Name)
		controlfbs.AllocationVariantStart(b)
		controlfbs.AllocationVariantAddName(b, name)
		controlfbs.AllocationVariantAddControl(b, v.Control)
		controlfbs.AllocationVariantAddWeightBp(b, v.WeightBP)
		variants[i] = controlfbs.AllocationVariantEnd(b)
	}
	vector := writeTables(b, variants, controlfbs.ExperimentAllocationStartVariantsVector)
	if a.Segment != nil {
		segment = writeRule(b, *a.Segment)
	}
	controlfbs.ExperimentAllocationStart(b)
	controlfbs.ExperimentAllocationAddKey(b, key)
	controlfbs.ExperimentAllocationAddScope(b, controlfbs.ExperimentScope(wireOf(ExperimentScopes(), a.Scope)))
	controlfbs.ExperimentAllocationAddSalt(b, salt)
	controlfbs.ExperimentAllocationAddVariants(b, vector)
	if layer != 0 {
		controlfbs.ExperimentAllocationAddLayer(b, layer)
		controlfbs.ExperimentAllocationAddLayerStart(b, a.LayerStart)
		controlfbs.ExperimentAllocationAddLayerEnd(b, a.LayerEnd)
	}
	controlfbs.ExperimentAllocationAddHoldoutBp(b, a.HoldoutBP)
	if segment != 0 {
		controlfbs.ExperimentAllocationAddSegment(b, segment)
	}
	controlfbs.ExperimentAllocationAddStartsAt(b, unixOrZero(a.StartsAt))
	controlfbs.ExperimentAllocationAddEndsAt(b, unixOrZero(a.EndsAt))
	return controlfbs.ExperimentAllocationEnd(b)
}

// unixOrZero is the wire form of an optional instant: zero for none.
func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
