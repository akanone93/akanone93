// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package controldoc

import (
	"fmt"
	"time"

	"github.com/nightCode42/plux3/backend/internal/controldoc/controlfbs"
)

// malformed reports a buffer that does not fit the schema.
func malformed(format string, args ...any) error {
	return fmt.Errorf("controldoc.Decode: %w: %s", ErrMalformed, fmt.Sprintf(format, args...))
}

// readExperimentation reads the flags, the experiments and the holdout into
// d. A vector longer than the buffer is malformed.
func readExperimentation(root *controlfbs.ControlDocument, d *Document) error {
	size := len(root.Table().Bytes)
	if n := root.FlagsLength(); n > size {
		return malformed("%d flags in a buffer of %d bytes", n, size)
	}
	var f controlfbs.FlagValue
	for i := range root.FlagsLength() {
		if !root.Flags(&f, i) {
			break
		}
		flag, err := readFlag(&f, size)
		if err != nil {
			return err
		}
		d.Flags = append(d.Flags, flag)
	}
	if n := root.ExperimentsLength(); n > size {
		return malformed("%d experiments in a buffer of %d bytes", n, size)
	}
	var a controlfbs.ExperimentAllocation
	for i := range root.ExperimentsLength() {
		if !root.Experiments(&a, i) {
			break
		}
		alloc, err := readAllocation(&a, size)
		if err != nil {
			return err
		}
		d.Experiments = append(d.Experiments, alloc)
	}
	if h := root.Holdout(nil); h != nil {
		d.Holdout = &Holdout{BasisPoints: h.BasisPoints(), Salt: string(h.Salt())}
	}
	return nil
}

func readFlag(f *controlfbs.FlagValue, size int) (FlagValue, error) {
	typ, ok := memberOf(FlagTypes(), byte(f.Type()))
	if !ok {
		return FlagValue{}, fmt.Errorf("controldoc.Decode: %w: flag type %s", ErrInvalid, f.Type())
	}
	out := FlagValue{Key: string(f.Key()), Type: typ, Default: string(f.DefaultValue())}
	if n := f.SegmentsLength(); n > size {
		return FlagValue{}, malformed("%d flag segments in a buffer of %d bytes", n, size)
	}
	var s controlfbs.FlagSegmentValue
	var r controlfbs.TargetingRule
	for i := range f.SegmentsLength() {
		if !f.Segments(&s, i) {
			break
		}
		rule, err := readRule(s.Rule(&r), 0, size)
		if err != nil {
			return FlagValue{}, err
		}
		out.Segments = append(out.Segments, FlagSegmentValue{Segment: string(s.Segment()), Rule: rule, Value: string(s.Value())})
	}
	return out, nil
}

// readRule reads a targeting rule, refusing one nested deeper than
// MaxRuleDepth or longer than the buffer.
func readRule(t *controlfbs.TargetingRule, depth, size int) (Rule, error) {
	if t == nil {
		return Rule{}, malformed("a targeting rule is missing")
	}
	if depth > MaxRuleDepth {
		return Rule{}, fmt.Errorf("controldoc.Decode: %w: a targeting rule nests deeper than %d groups", ErrInvalid, MaxRuleDepth)
	}
	out := Rule{Attribute: string(t.Attribute())}
	if out.Attribute != "" {
		var ok bool
		if out.Operator, ok = memberOf(ruleOperators, byte(t.Operator())); !ok {
			return Rule{}, fmt.Errorf("controldoc.Decode: %w: rule operator %s", ErrInvalid, t.Operator())
		}
		if out.ValueType, ok = memberOf(valueTypes, byte(t.ValueType())); !ok {
			return Rule{}, fmt.Errorf("controldoc.Decode: %w: rule value type %s", ErrInvalid, t.ValueType())
		}
	}
	if n := t.ValuesLength(); n > size || t.AllLength() > size || t.AnyLength() > size {
		return Rule{}, malformed("a rule longer than the buffer of %d bytes", size)
	}
	for i := range t.ValuesLength() {
		out.Values = append(out.Values, string(t.Values(i)))
	}
	var err error
	if out.All, err = readRules(t.AllLength(), t.All, depth, size); err != nil {
		return Rule{}, err
	}
	out.Any, err = readRules(t.AnyLength(), t.Any, depth, size)
	return out, err
}

// readRules reads the n rules a vector accessor yields.
func readRules(n int, at func(*controlfbs.TargetingRule, int) bool, depth, size int) ([]Rule, error) {
	var out []Rule
	for i := range n {
		var t controlfbs.TargetingRule
		if !at(&t, i) {
			break
		}
		rule, err := readRule(&t, depth+1, size)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, nil
}

func readAllocation(a *controlfbs.ExperimentAllocation, size int) (Allocation, error) {
	scope, ok := memberOf(ExperimentScopes(), byte(a.Scope()))
	if !ok {
		return Allocation{}, fmt.Errorf("controldoc.Decode: %w: experiment scope %s", ErrInvalid, a.Scope())
	}
	out := Allocation{
		Key: string(a.Key()), Scope: scope, Salt: string(a.Salt()), Layer: string(a.Layer()),
		LayerStart: a.LayerStart(), LayerEnd: a.LayerEnd(), HoldoutBP: a.HoldoutBp(),
	}
	if s := a.StartsAt(); s != 0 {
		out.StartsAt = time.Unix(s, 0).UTC()
	}
	if e := a.EndsAt(); e != 0 {
		out.EndsAt = time.Unix(e, 0).UTC()
	}
	if n := a.VariantsLength(); n > size {
		return Allocation{}, malformed("%d variants in a buffer of %d bytes", n, size)
	}
	var v controlfbs.AllocationVariant
	for i := range a.VariantsLength() {
		if !a.Variants(&v, i) {
			break
		}
		out.Variants = append(out.Variants, AllocationVariant{Name: string(v.Name()), Control: v.Control(), WeightBP: v.WeightBp()})
	}
	if seg := a.Segment(nil); seg != nil {
		rule, err := readRule(seg, 0, size)
		if err != nil {
			return Allocation{}, err
		}
		out.Segment = &rule
	}
	return out, nil
}
