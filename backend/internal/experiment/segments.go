// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
)

// SegmentSource resolves a segment key to the segment's targeting rule.
type SegmentSource interface {
	// Rule returns the rule of the segment with the key in the app, or
	// PLX-1102 when the app has no such segment.
	Rule(ctx context.Context, p auth.Principal, appID, key string) (controldoc.Rule, error)
}

// draftSegments reads segments from the app's draft: the segment documents
// of the project, `segments/<key>.segment.json`.
type draftSegments struct {
	docs   Documents
	loader *schema.Loader
}

// Rule implements SegmentSource.
func (d draftSegments) Rule(ctx context.Context, p auth.Principal, appID, key string) (controldoc.Rule, error) {
	if !schema.ValidKey(key) {
		return controldoc.Rule{}, plxerr.New(plxerr.InvalidFormat, "%q is not a segment key", key)
	}
	if d.docs == nil {
		return controldoc.Rule{}, errors.New("experiment: no document service to read segments from")
	}
	path := "segments/" + key + ".segment.json"
	doc, err := d.docs.GetDocument(ctx, p, appID, "", path)
	if code, ok := plxerr.CodeOf(err); ok && code == plxerr.ResourceNotFound {
		return controldoc.Rule{}, plxerr.New(plxerr.UnresolvedReference, "the app defines no segment %q", key)
	}
	if err != nil {
		return controldoc.Rule{}, fmt.Errorf("experiment: read segment %q: %w", key, err)
	}
	src, diags := d.loader.ParseDocument(path, doc.Content, schema.KindSegment)
	if diags.HasErrors() {
		return controldoc.Rule{}, refusal(diags)
	}
	var seg schema.SegmentDocument
	if err := json.Unmarshal(src.Canonical, &seg); err != nil {
		return controldoc.Rule{}, fmt.Errorf("experiment: read segment %q: %w", key, err)
	}
	return RuleOf(seg.Rule)
}

// refusal turns the first error of a document's diagnostics into the
// refusal a caller understands.
func refusal(diags plxerr.Diagnostics) error {
	for _, d := range diags {
		if d.Severity == plxerr.SeverityError {
			e := &plxerr.Error{Code: d.Code, Message: d.Message}
			return e.WithDetail("file", d.File).WithDetail("path", d.Path)
		}
	}
	return plxerr.New(plxerr.InvalidStructure, "the document could not be read")
}

// builtinAttributes are the targeting attributes every device has, with
// whether their values are versions (REL-051).
var builtinAttributes = map[string]bool{
	"build": false, "runtimeVersion": true, "os": false, "osVersion": true,
	"deviceClass": false, "locale": false, "region": false, "channel": false,
}

// versionPattern is a version: one to three dot-separated numbers.
var versionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)

// RuleOf converts the rule of a segment document into the targeting rule of
// the control document. It checks what the control document needs a device to
// evaluate: a known attribute, an operator that suits the operands, and
// operands of one type. It reports PLX-1704 for a rule it cannot convert.
func RuleOf(r schema.SegmentRule) (controldoc.Rule, error) {
	return ruleOf(r, 0)
}

func ruleOf(r schema.SegmentRule, depth int) (controldoc.Rule, error) {
	if depth > controldoc.MaxRuleDepth {
		return controldoc.Rule{}, plxerr.New(plxerr.SegmentRuleInvalid, "a segment rule nests deeper than %d groups", controldoc.MaxRuleDepth)
	}
	if len(r.All) > 0 || len(r.Any) > 0 {
		if r.Attribute != "" {
			return controldoc.Rule{}, plxerr.New(plxerr.SegmentRuleInvalid, "a segment rule is a condition or a group, not both")
		}
		all, err := rulesOf(r.All, depth)
		if err != nil {
			return controldoc.Rule{}, err
		}
		anyOf, err := rulesOf(r.Any, depth)
		return controldoc.Rule{All: all, Any: anyOf}, err
	}
	return conditionOf(r)
}

func rulesOf(in []schema.SegmentRule, depth int) ([]controldoc.Rule, error) {
	var out []controldoc.Rule
	for _, g := range in {
		rule, err := ruleOf(g, depth+1)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, nil
}

// conditionOf converts one condition.
func conditionOf(r schema.SegmentRule) (controldoc.Rule, error) {
	versioned, builtin := builtinAttributes[r.Attribute]
	if _, user := strings.CutPrefix(r.Attribute, "user."); !builtin && !user {
		return controldoc.Rule{}, plxerr.New(plxerr.SegmentRuleInvalid, "%q is not a targeting attribute: use a built-in attribute or user.<name>", r.Attribute)
	}
	if !r.Operator.Valid() {
		return controldoc.Rule{}, plxerr.New(plxerr.SegmentRuleInvalid, "%q is not a rule operator", r.Operator)
	}
	typ, values, err := operandsOf(r)
	if err != nil {
		return controldoc.Rule{}, err
	}
	ordered := r.Operator == schema.SegmentOperatorLt || r.Operator == schema.SegmentOperatorLte ||
		r.Operator == schema.SegmentOperatorGt || r.Operator == schema.SegmentOperatorGte
	switch {
	case ordered && typ == controldoc.ValueBool, ordered && typ == controldoc.ValueString && !versioned:
		return controldoc.Rule{}, plxerr.New(plxerr.SegmentRuleInvalid, "%q does not order the values of %s", r.Operator, r.Attribute)
	case versioned:
		for _, v := range values {
			if typ != controldoc.ValueString || !versionPattern.MatchString(v) {
				return controldoc.Rule{}, plxerr.New(plxerr.SegmentRuleInvalid, "%q is not a version for %s", v, r.Attribute)
			}
		}
	}
	return controldoc.Rule{Attribute: r.Attribute, Operator: controldoc.RuleOperator(r.Operator), ValueType: typ, Values: values}, nil
}

// operandsOf reads the operands of a condition: one scalar, or a non-empty
// list of scalars of one type for in and notIn. Numbers are written in their
// canonical text, so equal rules have equal bytes.
func operandsOf(r schema.SegmentRule) (controldoc.ValueType, []string, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(r.Value))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return "", nil, plxerr.New(plxerr.SegmentRuleInvalid, "the value of a condition on %s is not JSON", r.Attribute)
	}
	list, isList := v.([]any)
	set := r.Operator == schema.SegmentOperatorIn || r.Operator == schema.SegmentOperatorNotIn
	switch {
	case set && (!isList || len(list) == 0):
		return "", nil, plxerr.New(plxerr.SegmentRuleInvalid, "%q takes a non-empty list of values", r.Operator)
	case !set && isList:
		return "", nil, plxerr.New(plxerr.SegmentRuleInvalid, "%q takes one value, not a list", r.Operator)
	case !set:
		list = []any{v}
	}
	var typ controldoc.ValueType
	values := make([]string, 0, len(list))
	for _, e := range list {
		et, text, err := operandOf(e)
		if err != nil {
			return "", nil, err
		}
		if typ != "" && et != typ {
			return "", nil, plxerr.New(plxerr.SegmentRuleInvalid, "the values of a condition on %s are of more than one type", r.Attribute)
		}
		typ = et
		values = append(values, text)
	}
	return typ, values, nil
}

// operandOf reads one operand and writes it as text.
func operandOf(v any) (controldoc.ValueType, string, error) {
	switch x := v.(type) {
	case string:
		return controldoc.ValueString, x, nil
	case bool:
		return controldoc.ValueBool, fmt.Sprint(x), nil
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return "", "", plxerr.New(plxerr.SegmentRuleInvalid, "%s is not a number a device can compare", x)
		}
		text, err := jcs.FormatNumber(f)
		if err != nil {
			return "", "", plxerr.New(plxerr.SegmentRuleInvalid, "%s is not a number a device can compare", x)
		}
		return controldoc.ValueNumber, text, nil
	}
	return "", "", plxerr.New(plxerr.SegmentRuleInvalid, "a rule value is a string, a number or a boolean")
}
