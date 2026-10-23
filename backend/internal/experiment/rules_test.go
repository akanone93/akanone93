// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

var epoch = time.Unix(1_800_000_000, 0).UTC()

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func cond(attr string, op schema.SegmentOperator, value string) schema.SegmentRule {
	return schema.SegmentRule{Attribute: attr, Operator: op, Value: json.RawMessage(value)}
}

// Verifies: ABT-001, ABT-006, REL-051.
// A segment's rule becomes the control document's rule, with numbers in their
// canonical text and groups kept in order.
func TestRuleOfConvertsSegments_ABT_006(t *testing.T) {
	t.Parallel()
	got, err := RuleOf(schema.SegmentRule{All: []schema.SegmentRule{
		cond("os", schema.SegmentOperatorEq, `"ios"`),
		{Any: []schema.SegmentRule{
			cond("build", schema.SegmentOperatorGte, `120.0`),
			cond("locale", schema.SegmentOperatorIn, `["de","de-AT"]`),
			cond("runtimeVersion", schema.SegmentOperatorLt, `"1.4.0"`),
			cond("user.vip", schema.SegmentOperatorEq, `true`),
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := controldoc.Rule{All: []controldoc.Rule{
		{Attribute: "os", Operator: controldoc.OpEq, ValueType: controldoc.ValueString, Values: []string{"ios"}},
		{Any: []controldoc.Rule{
			{Attribute: "build", Operator: controldoc.OpGte, ValueType: controldoc.ValueNumber, Values: []string{"120"}},
			{Attribute: "locale", Operator: controldoc.OpIn, ValueType: controldoc.ValueString, Values: []string{"de", "de-AT"}},
			{Attribute: "runtimeVersion", Operator: controldoc.OpLt, ValueType: controldoc.ValueString, Values: []string{"1.4.0"}},
			{Attribute: "user.vip", Operator: controldoc.OpEq, ValueType: controldoc.ValueBool, Values: []string{"true"}},
		}},
	}}
	if a, b := mustMarshal(t, got), mustMarshal(t, want); a != b {
		t.Fatalf("rule:\n got %s\nwant %s", a, b)
	}
	if _, err := controldoc.Encode(controldoc.Document{
		Version: 1, AppID: "a", Environment: "e", Channel: "c", IssuedAt: epoch, ExpiresAt: epoch.Add(1),
		Flags: []controldoc.FlagValue{{Key: "k", Type: controldoc.FlagBool, Default: "true", Segments: []controldoc.FlagSegmentValue{{Segment: "s", Rule: got, Value: "false"}}}},
	}); err != nil {
		t.Fatalf("the converted rule does not fit the control document: %v", err)
	}
}

// Verifies: ABT-001, ABT-006, REL-051.
// A rule a device could not evaluate is refused with PLX-1704.
func TestRuleOfRefusals_ABT_006(t *testing.T) {
	t.Parallel()
	deep := cond("os", schema.SegmentOperatorEq, `"ios"`)
	for range controldoc.MaxRuleDepth + 2 {
		deep = schema.SegmentRule{All: []schema.SegmentRule{deep}}
	}
	for name, rule := range map[string]schema.SegmentRule{
		"unknown attribute":     cond("shoeSize", schema.SegmentOperatorEq, `"9"`),
		"unknown operator":      cond("os", "like", `"ios"`),
		"list for eq":           cond("os", schema.SegmentOperatorEq, `["ios"]`),
		"scalar for in":         cond("os", schema.SegmentOperatorIn, `"ios"`),
		"empty list":            cond("os", schema.SegmentOperatorIn, `[]`),
		"mixed types":           cond("user.x", schema.SegmentOperatorIn, `["a", 1]`),
		"object operand":        cond("user.x", schema.SegmentOperatorEq, `{"a":1}`),
		"null operand":          cond("user.x", schema.SegmentOperatorEq, `null`),
		"not json":              cond("os", schema.SegmentOperatorEq, `{`),
		"order on a string":     cond("os", schema.SegmentOperatorGt, `"ios"`),
		"order on a bool":       cond("user.x", schema.SegmentOperatorLt, `true`),
		"version not a version": cond("runtimeVersion", schema.SegmentOperatorGte, `"new"`),
		"version as a number":   cond("osVersion", schema.SegmentOperatorGte, `17`),
		"condition and group":   {Attribute: "os", Operator: schema.SegmentOperatorEq, Value: json.RawMessage(`"ios"`), All: []schema.SegmentRule{deep}},
		"too deep":              deep,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := RuleOf(rule)
			if code, ok := plxerr.CodeOf(err); !ok || code != plxerr.SegmentRuleInvalid {
				t.Fatalf("RuleOf = %v, want PLX-1704", err)
			}
		})
	}
}

// Verifies: ABT-004.
// The layer allocator gives the first free range, keeps running experiments
// where they are and refuses when no single range is large enough, naming the
// experiments that hold the layer (PLX-8200).
func TestAllocateLayerRanges_ABT_004(t *testing.T) {
	t.Parallel()
	got, err := allocate("l", nil, 10000)
	if err != nil || got.start != 0 || got.end != 10000 {
		t.Fatalf("empty layer: %+v, %v", got, err)
	}
	held := []span{{start: 6000, end: 7000, key: "c"}, {start: 0, end: 2000, key: "a"}, {start: 2000, end: 3000, key: "b"}}
	for _, c := range []struct {
		size       uint32
		start, end uint32
	}{{1000, 3000, 4000}, {3000, 3000, 6000}} {
		got, err := allocate("l", held, c.size)
		if err != nil || got.start != c.start || got.end != c.end {
			t.Errorf("allocate(%d) = %+v, %v; want %d-%d", c.size, got, err, c.start, c.end)
		}
	}
	_, err = allocate("l", held, 3001)
	var e *plxerr.Error
	if !errors.As(err, &e) || e.Code != plxerr.ExperimentOverlapsLayer {
		t.Fatalf("allocate(4000) = %v, want PLX-8200", err)
	}
	if !strings.Contains(e.Message, "[a b c]") {
		t.Errorf("the refusal does not name who holds the layer: %s", e.Message)
	}
	got, err = allocate("l", []span{{start: 0, end: 10000, key: "all"}}, 1)
	if err == nil {
		t.Fatalf("a full layer gave %+v", got)
	}
}

// Verifies: ABT-004.
// A share of a layer is above 0 and at most 100 percent with two decimals.
func TestShareBP_ABT_004(t *testing.T) {
	t.Parallel()
	for in, want := range map[float64]uint32{100: 10000, 0.01: 1, 33.33: 3333, 12.5: 1250} {
		if got, err := shareBP(in); err != nil || got != want {
			t.Errorf("shareBP(%v) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []float64{0, -1, 100.01, 0.001, 33.333} {
		if _, err := shareBP(in); err == nil {
			t.Errorf("shareBP(%v) was accepted", in)
		}
	}
}
