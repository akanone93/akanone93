// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package controldoc

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// RuleOperator is how a condition compares an attribute with its operands.
type RuleOperator string

// The operators of a condition. The spellings are those of the segment
// document; the wire form is controlfbs.RuleOperator.
const (
	OpEq    RuleOperator = "eq"
	OpNeq   RuleOperator = "neq"
	OpIn    RuleOperator = "in"
	OpNotIn RuleOperator = "notIn"
	OpLt    RuleOperator = "lt"
	OpLte   RuleOperator = "lte"
	OpGt    RuleOperator = "gt"
	OpGte   RuleOperator = "gte"
)

// ValueType is the type of the operands of a condition.
type ValueType string

// The operand types; the wire form is controlfbs.RuleValueType.
const (
	ValueString ValueType = "string"
	ValueNumber ValueType = "number"
	ValueBool   ValueType = "bool"
)

// MaxRuleDepth bounds how deeply the groups of a targeting rule nest. It is
// a structural bound that keeps encoding and decoding of the recursive wire
// table finite, not a limit a tenant may tune.
const MaxRuleDepth = 16

// Rule is a targeting rule (REL-051): a condition when Attribute is set and
// a group of rules combined with All or Any otherwise.
type Rule struct {
	Attribute string       `json:"attribute,omitempty"`
	Operator  RuleOperator `json:"operator,omitempty"`
	ValueType ValueType    `json:"valueType,omitempty"`
	// Values are the operands in canonical text: one, or at least one for
	// In and NotIn.
	Values []string `json:"values,omitempty"`
	All    []Rule   `json:"all,omitempty"`
	Any    []Rule   `json:"any,omitempty"`
}

// FlagType is the type of a feature flag (ABT-006).
type FlagType string

// The flag types; the wire form is controlfbs.FlagType.
const (
	FlagBool   FlagType = "bool"
	FlagNumber FlagType = "number"
	FlagString FlagType = "string"
	FlagJSON   FlagType = "json"
)

// FlagTypes lists every flag type in the order of the wire enumeration.
func FlagTypes() []FlagType { return []FlagType{FlagBool, FlagNumber, FlagString, FlagJSON} }

// FlagSegmentValue is the value of a flag for the devices of one segment.
type FlagSegmentValue struct {
	// Segment is the segment's key, for reports.
	Segment string `json:"segment"`
	// Rule is the segment's rule as it was when the value was written.
	Rule Rule `json:"rule"`
	// Value is canonical JSON text.
	Value string `json:"value"`
}

// FlagValue is one feature flag with its default and its per-segment
// values, first match first.
type FlagValue struct {
	Key  string   `json:"key"`
	Type FlagType `json:"type"`
	// Default is canonical JSON text.
	Default  string             `json:"default"`
	Segments []FlagSegmentValue `json:"segments,omitempty"`
}

// ExperimentScope is what an experiment varies (ABT-001).
type ExperimentScope string

// The scopes of an experiment; the spellings are those of the experiment
// document and the wire form is controlfbs.ExperimentScope.
const (
	ScopeExperimentPage          ExperimentScope = "page"
	ScopeExperimentSubtree       ExperimentScope = "subtree"
	ScopeExperimentProp          ExperimentScope = "prop"
	ScopeExperimentFlow          ExperimentScope = "flow"
	ScopeExperimentPluginVersion ExperimentScope = "pluginVersion"
)

// ExperimentScopes lists every experiment scope in wire order.
func ExperimentScopes() []ExperimentScope {
	return []ExperimentScope{
		ScopeExperimentPage, ScopeExperimentSubtree, ScopeExperimentProp, ScopeExperimentFlow, ScopeExperimentPluginVersion,
	}
}

// BucketCount is the number of buckets subjects are hashed into: weights,
// holdouts and layer ranges are counted in basis points of it.
const BucketCount = 10000

// AllocationVariant is a variant with its share in basis points.
type AllocationVariant struct {
	Name     string `json:"name"`
	Control  bool   `json:"control,omitempty"`
	WeightBP uint32 `json:"weightBp"`
}

// Allocation is the allocation of one running experiment (ABT-001,
// ABT-004); the assignment procedure is in schema/control.fbs.
type Allocation struct {
	Key   string          `json:"key"`
	Scope ExperimentScope `json:"scope"`
	Salt  string          `json:"salt"`
	// Variants are in the order of the experiment document.
	Variants []AllocationVariant `json:"variants"`
	// Layer is empty outside a layer; LayerStart and LayerEnd then are zero.
	Layer      string `json:"layer,omitempty"`
	LayerStart uint32 `json:"layerStart,omitempty"`
	LayerEnd   uint32 `json:"layerEnd,omitempty"`
	HoldoutBP  uint32 `json:"holdoutBp,omitempty"`
	// Segment is nil for everyone.
	Segment *Rule `json:"segment,omitempty"`
	// StartsAt and EndsAt bound the experiment; the zero time is unbounded.
	StartsAt time.Time `json:"startsAt,omitzero"`
	EndsAt   time.Time `json:"endsAt,omitzero"`
}

// Holdout is the share kept out of every experiment (ABT-004).
type Holdout struct {
	BasisPoints uint32 `json:"basisPoints"`
	Salt        string `json:"salt"`
}

// validate checks a rule: the shape of a condition or a group, known
// operators and types, and a bounded depth.
func (r Rule) validate(depth int) error {
	if depth > MaxRuleDepth {
		return fmt.Errorf("%w: a targeting rule nests deeper than %d groups", ErrInvalid, MaxRuleDepth)
	}
	groups := len(r.All) + len(r.Any)
	switch {
	case r.Attribute != "" && groups > 0:
		return fmt.Errorf("%w: a rule is a condition or a group, not both", ErrInvalid)
	case r.Attribute == "" && groups == 0:
		return fmt.Errorf("%w: a rule needs an attribute or a group", ErrInvalid)
	case r.Attribute == "":
		return r.validateGroups(depth)
	}
	return r.validateCondition()
}

func (r Rule) validateGroups(depth int) error {
	for _, g := range slices.Concat(r.All, r.Any) {
		if err := g.validate(depth + 1); err != nil {
			return err
		}
	}
	return nil
}

func (r Rule) validateCondition() error {
	if len(r.Attribute) > MaxText {
		return fmt.Errorf("%w: the attribute %q is too long", ErrInvalid, r.Attribute)
	}
	if !slices.Contains(ruleOperators, r.Operator) {
		return fmt.Errorf("%w: %q is not a rule operator", ErrInvalid, r.Operator)
	}
	if !slices.Contains(valueTypes, r.ValueType) {
		return fmt.Errorf("%w: %q is not a rule value type", ErrInvalid, r.ValueType)
	}
	multi := r.Operator == OpIn || r.Operator == OpNotIn
	switch {
	case multi && len(r.Values) == 0:
		return fmt.Errorf("%w: operator %s takes one or more operands", ErrInvalid, r.Operator)
	case !multi && len(r.Values) != 1:
		return fmt.Errorf("%w: operator %s takes exactly one operand", ErrInvalid, r.Operator)
	}
	return nil
}

// ruleOperators and valueTypes list the members in wire order.
var (
	ruleOperators = []RuleOperator{OpEq, OpNeq, OpIn, OpNotIn, OpLt, OpLte, OpGt, OpGte}
	valueTypes    = []ValueType{ValueString, ValueNumber, ValueBool}
)

// validate checks a flag.
func (f FlagValue) validate() error {
	if !validText(f.Key) {
		return fmt.Errorf("%w: %q is not a flag key", ErrInvalid, f.Key)
	}
	if !slices.Contains(FlagTypes(), f.Type) {
		return fmt.Errorf("%w: flag %q has type %q", ErrInvalid, f.Key, f.Type)
	}
	if !json.Valid([]byte(f.Default)) {
		return fmt.Errorf("%w: the default of flag %q is not JSON", ErrInvalid, f.Key)
	}
	for _, s := range f.Segments {
		if !validText(s.Segment) || !json.Valid([]byte(s.Value)) {
			return fmt.Errorf("%w: flag %q has a malformed value for segment %q", ErrInvalid, f.Key, s.Segment)
		}
		if err := s.Rule.validate(0); err != nil {
			return err
		}
	}
	return nil
}

// validate checks an allocation.
func (a Allocation) validate() error {
	if !validText(a.Key) || a.Salt == "" || len(a.Salt) > MaxText {
		return fmt.Errorf("%w: experiment %q needs a key and a salt", ErrInvalid, a.Key)
	}
	if !slices.Contains(ExperimentScopes(), a.Scope) {
		return fmt.Errorf("%w: experiment %q has scope %q", ErrInvalid, a.Key, a.Scope)
	}
	if err := a.validateVariants(); err != nil {
		return err
	}
	if a.HoldoutBP > BucketCount || a.LayerEnd > BucketCount || a.LayerStart > a.LayerEnd || (a.Layer == "") != (a.LayerEnd == 0) {
		return fmt.Errorf("%w: experiment %q has a holdout or a layer range outside 0 to %d", ErrInvalid, a.Key, BucketCount)
	}
	if a.Segment != nil {
		return a.Segment.validate(0)
	}
	return nil
}

func (a Allocation) validateVariants() error {
	var total uint64
	controls := 0
	for _, v := range a.Variants {
		if !validText(v.Name) {
			return fmt.Errorf("%w: experiment %q has a variant named %q", ErrInvalid, a.Key, v.Name)
		}
		total += uint64(v.WeightBP)
		if v.Control {
			controls++
		}
	}
	if len(a.Variants) < 2 || controls != 1 || total != BucketCount {
		return fmt.Errorf("%w: experiment %q needs at least two variants, one control, and weights that sum to %d", ErrInvalid, a.Key, BucketCount)
	}
	return nil
}

// ValidateExperimentation checks flags, experiments and a holdout against the
// rules of the schema, so that a caller can refuse them before they are
// stored rather than when the document is signed.
func ValidateExperimentation(flags []FlagValue, experiments []Allocation, holdout *Holdout) error {
	return Document{Flags: flags, Experiments: experiments, Holdout: holdout}.validateExperimentation()
}

// validateExperimentation checks the flags, experiments and holdout.
func (d Document) validateExperimentation() error {
	keys := map[string]bool{}
	for _, f := range d.Flags {
		if err := f.validate(); err != nil {
			return err
		}
		if keys[f.Key] {
			return fmt.Errorf("%w: flag %q is listed twice", ErrInvalid, f.Key)
		}
		keys[f.Key] = true
	}
	clear(keys)
	for _, a := range d.Experiments {
		if err := a.validate(); err != nil {
			return err
		}
		if keys[a.Key] {
			return fmt.Errorf("%w: experiment %q is listed twice", ErrInvalid, a.Key)
		}
		keys[a.Key] = true
	}
	if h := d.Holdout; h != nil && (h.BasisPoints > BucketCount || h.Salt == "" || len(h.Salt) > MaxText) {
		return fmt.Errorf("%w: the global holdout needs a salt and a share of at most %d basis points", ErrInvalid, BucketCount)
	}
	return nil
}
