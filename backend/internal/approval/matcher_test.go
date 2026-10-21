// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"testing"

	"github.com/nightCode42/plux3/backend/internal/schema"
)

// boolPtr returns a pointer to b.
func boolPtr(b bool) *bool { return &b }

// release is a production release promotion that touches a secure page.
func release() Artifact {
	return Artifact{
		Resource: schema.PolicyResourceRelease, Action: ActionPromote,
		Environment: Environment{ID: "e1", Key: "production", Production: true},
		Risk:        []schema.PolicyRisk{schema.PolicyRiskTouchesSecurePage},
	}
}

// Verifies: GOV-020.
func TestMatches_GOV_020(t *testing.T) {
	cases := []struct {
		name  string
		match schema.PolicyMatch
		art   Artifact
		want  bool
	}{
		{"an empty match matches anything", schema.PolicyMatch{}, release(), true},
		{"resource listed", schema.PolicyMatch{Resources: []schema.PolicyResource{schema.PolicyResourceRelease}}, release(), true},
		{"resource not listed", schema.PolicyMatch{Resources: []schema.PolicyResource{schema.PolicyResourceWaiver}}, release(), false},
		{"environment key listed", schema.PolicyMatch{Environments: []string{"staging", "production"}}, release(), true},
		{"environment key not listed", schema.PolicyMatch{Environments: []string{"staging"}}, release(), false},
		{"environments never match a change outside one", schema.PolicyMatch{Environments: []string{"production"}}, Artifact{Resource: schema.PolicyResourceLimit}, false},
		{"action listed", schema.PolicyMatch{Actions: []string{ActionPromote}}, release(), true},
		{"action not listed", schema.PolicyMatch{Actions: []string{ActionDeploy}}, release(), false},
		{"production true on production", schema.PolicyMatch{Production: boolPtr(true)}, release(), true},
		{"production true on another", schema.PolicyMatch{Production: boolPtr(true)}, Artifact{Environment: Environment{Key: "dev"}}, false},
		{"production false on production", schema.PolicyMatch{Production: boolPtr(false)}, release(), false},
		{"production false on another", schema.PolicyMatch{Production: boolPtr(false)}, Artifact{Environment: Environment{Key: "dev"}}, true},
		{"risk held", schema.PolicyMatch{Risk: []schema.PolicyRisk{schema.PolicyRiskTouchesSecurePage}}, release(), true},
		{"risk not held", schema.PolicyMatch{Risk: []schema.PolicyRisk{schema.PolicyRiskTouchesMoneyFlow}}, release(), false},
		{"every risk must hold", schema.PolicyMatch{Risk: []schema.PolicyRisk{schema.PolicyRiskTouchesSecurePage, schema.PolicyRiskTouchesMoneyFlow}}, release(), false},
		{"every attribute must hold", schema.PolicyMatch{
			Resources: []schema.PolicyResource{schema.PolicyResourceRelease}, Environments: []string{"production"},
			Actions: []string{ActionPromote}, Production: boolPtr(true), Risk: []schema.PolicyRisk{schema.PolicyRiskTouchesSecurePage},
		}, release(), true},
		{"one attribute failing fails the match", schema.PolicyMatch{
			Resources: []schema.PolicyResource{schema.PolicyResourceRelease}, Actions: []string{ActionDeploy},
		}, release(), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Matches(c.match, c.art); got != c.want {
				t.Fatalf("Matches = %v, want %v", got, c.want)
			}
		})
	}
}

// Verifies: GOV-022.
func TestHashOf_GOV_022(t *testing.T) {
	if HashOf([]byte("ab"), []byte("c")) == HashOf([]byte("a"), []byte("bc")) {
		t.Fatal("moving content between parts must change the hash")
	}
	if HashOf([]byte("x")) != HashOf([]byte("x")) {
		t.Fatal("the same content must hash the same")
	}
	a, err := HashJSON(map[string]any{"b": 1, "a": []int{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashJSON(map[string]any{"a": []int{1, 2}, "b": 1})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("key order must not change the hash")
	}
}
