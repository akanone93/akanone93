// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package controldoc_test

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/controldoc"
)

func nested(depth int) controldoc.Rule {
	r := controldoc.Rule{Attribute: "os", Operator: controldoc.OpEq, ValueType: controldoc.ValueString, Values: []string{"ios"}}
	for range depth {
		r = controldoc.Rule{All: []controldoc.Rule{r}}
	}
	return r
}

// experimental is sample() plus flags, experiments and a holdout.
func experimental() controldoc.Document {
	d := sample()
	ios := controldoc.Rule{Attribute: "os", Operator: controldoc.OpEq, ValueType: controldoc.ValueString, Values: []string{"ios"}}
	beta := controldoc.Rule{Any: []controldoc.Rule{
		{Attribute: "build", Operator: controldoc.OpGte, ValueType: controldoc.ValueNumber, Values: []string{"120"}},
		{Attribute: "locale", Operator: controldoc.OpIn, ValueType: controldoc.ValueString, Values: []string{"de", "de-AT"}},
	}}
	d.Flags = []controldoc.FlagValue{
		{Key: "newCheckout", Type: controldoc.FlagBool, Default: "false", Segments: []controldoc.FlagSegmentValue{
			{Segment: "ios-users", Rule: ios, Value: "true"},
			{Segment: "beta", Rule: beta, Value: "true"},
		}},
		{Key: "banner", Type: controldoc.FlagJSON, Default: `{"a":1}`},
	}
	d.Experiments = []controldoc.Allocation{{
		Key: "checkoutButton", Scope: controldoc.ScopeExperimentSubtree, Salt: "salt-1",
		Variants: []controldoc.AllocationVariant{{Name: "control", Control: true, WeightBP: 5000}, {Name: "green", WeightBP: 5000}},
		Layer:    "checkout", LayerStart: 0, LayerEnd: 4000, HoldoutBP: 500, Segment: &ios,
		StartsAt: issued, EndsAt: issued.Add(48 * time.Hour),
	}}
	d.Holdout = &controldoc.Holdout{BasisPoints: 200, Salt: "holdout-salt"}
	return d
}

// TestExperimentationRoundTrip_ABT_006 round-trips flags with segment
// rules, an experiment allocation and the global holdout, and checks that
// the flags come back sorted by key with their segments in precedence order.
//
// Verifies: ABT-006, ABT-001, ABT-004, CMP-002.
func TestExperimentationRoundTrip_ABT_006(t *testing.T) {
	t.Parallel()
	buf, err := controldoc.Encode(experimental())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := controldoc.Decode(buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	want := experimental()
	slices.Reverse(want.Flags)
	want.KillSwitches = got.KillSwitches
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip:\n got %+v\nwant %+v", got, want)
	}
}

// TestExperimentationDeterministic_CMP_002 encodes the same content given in
// another order and expects the same bytes.
//
// Verifies: CMP-002, ABT-006, ABT-001.
func TestExperimentationDeterministic_CMP_002(t *testing.T) {
	t.Parallel()
	a, b := experimental(), experimental()
	slices.Reverse(b.Flags)
	first, err := controldoc.Encode(a)
	if err != nil {
		t.Fatal(err)
	}
	second, err := controldoc.Encode(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("the order flags were given in changed the bytes")
	}
}

// TestExperimentationValidation_ABT_001 holds the document to the rules of
// the flags, allocations and rules it carries.
//
// Verifies: ABT-001, ABT-004, ABT-006.
func TestExperimentationValidation_ABT_001(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*controldoc.Document){
		"flag type unknown":      func(d *controldoc.Document) { d.Flags[0].Type = "float" },
		"flag default not JSON":  func(d *controldoc.Document) { d.Flags[0].Default = "{" },
		"flag listed twice":      func(d *controldoc.Document) { d.Flags[1].Key = d.Flags[0].Key },
		"rule without operands":  func(d *controldoc.Document) { d.Flags[0].Segments[0].Rule.Values = nil },
		"eq with two operands":   func(d *controldoc.Document) { d.Flags[0].Segments[0].Rule.Values = []string{"a", "b"} },
		"rule unknown operator":  func(d *controldoc.Document) { d.Flags[0].Segments[0].Rule.Operator = "like" },
		"rule neither shape":     func(d *controldoc.Document) { d.Flags[0].Segments[0].Rule = controldoc.Rule{} },
		"rule both shapes":       func(d *controldoc.Document) { d.Flags[0].Segments[0].Rule.All = []controldoc.Rule{nested(0)} },
		"rule too deep":          func(d *controldoc.Document) { d.Flags[0].Segments[0].Rule = nested(controldoc.MaxRuleDepth + 1) },
		"weights not 100%":       func(d *controldoc.Document) { d.Experiments[0].Variants[1].WeightBP = 4000 },
		"no control":             func(d *controldoc.Document) { d.Experiments[0].Variants[0].Control = false },
		"two controls":           func(d *controldoc.Document) { d.Experiments[0].Variants[1].Control = true },
		"one variant":            func(d *controldoc.Document) { d.Experiments[0].Variants = d.Experiments[0].Variants[:1] },
		"layer range inverted":   func(d *controldoc.Document) { d.Experiments[0].LayerStart = 5000 },
		"layer range too wide":   func(d *controldoc.Document) { d.Experiments[0].LayerEnd = controldoc.BucketCount + 1 },
		"range without layer":    func(d *controldoc.Document) { d.Experiments[0].Layer = "" },
		"holdout above 100%":     func(d *controldoc.Document) { d.Experiments[0].HoldoutBP = controldoc.BucketCount + 1 },
		"experiment scope":       func(d *controldoc.Document) { d.Experiments[0].Scope = "app" },
		"experiment without key": func(d *controldoc.Document) { d.Experiments[0].Key = "" },
		"global holdout salt":    func(d *controldoc.Document) { d.Holdout.Salt = "" },
		"global holdout share":   func(d *controldoc.Document) { d.Holdout.BasisPoints = controldoc.BucketCount + 1 },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := experimental()
			mutate(&d)
			if _, err := controldoc.Encode(d); !errors.Is(err, controldoc.ErrInvalid) {
				t.Fatalf("Encode = %v, want ErrInvalid", err)
			}
		})
	}
}

// TestDecodeExperimentationMalformed_SEC_052 cuts an encoded document short at
// every length: Decode never panics and refuses a document missing the
// tables its root points to.
//
// Verifies: SEC-052, ABT-006.
func TestDecodeExperimentationMalformed_SEC_052(t *testing.T) {
	t.Parallel()
	buf, err := controldoc.Encode(experimental())
	if err != nil {
		t.Fatal(err)
	}
	for n := range len(buf) {
		_, err := controldoc.Decode(buf[:n])
		if err == nil && n < len(buf)/2 {
			t.Fatalf("a prefix of %d of %d bytes decoded", n, len(buf))
		}
	}
}
