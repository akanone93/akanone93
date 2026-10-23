// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/schema"
)

var epoch = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// policyOf builds a policy requiring req.
func policyWith(req schema.PolicyRequire) Policy {
	return Policy{Doc: schema.PolicyDocument{Key: "p", Require: req}}
}

// approval is an approving decision by who, holding roles and teams, i minutes after the epoch.
func approval(who string, i int, roles []string, teams ...string) Decision {
	return Decision{DeciderID: who, Roles: roles, Teams: teams, Approve: true, At: epoch.Add(time.Duration(i) * time.Minute)}
}

// Verifies: GOV-021.
func TestEvaluate_GOV_021(t *testing.T) {
	yes := true
	cases := []struct {
		name      string
		req       schema.PolicyRequire
		decisions []Decision
		excluded  []string
		want      bool
	}{
		{"no decisions", schema.PolicyRequire{Approvers: 1}, nil, nil, false},
		{"one approver is enough for one", schema.PolicyRequire{Approvers: 1}, []Decision{approval("a", 0, nil)}, nil, true},
		{"four-eyes needs two", schema.PolicyRequire{Approvers: 2}, []Decision{approval("a", 0, nil)}, nil, false},
		{"two approvers", schema.PolicyRequire{Approvers: 2}, []Decision{approval("a", 0, nil), approval("b", 1, nil)}, nil, true},
		{"the author's approval does not count", schema.PolicyRequire{Approvers: 2}, []Decision{approval("author", 0, nil), approval("b", 1, nil)}, []string{"author"}, false},
		{"roles accept members", schema.PolicyRequire{Approvers: 1, Roles: []string{"security"}}, []Decision{approval("a", 0, []string{"security"})}, nil, true},
		{"roles refuse others", schema.PolicyRequire{Approvers: 1, Roles: []string{"security"}}, []Decision{approval("a", 0, []string{"developer"})}, nil, false},
		{"named approvers", schema.PolicyRequire{Approvers: 1, Users: []string{"carol"}}, []Decision{approval("carol", 0, nil)}, nil, true},
		{"named approvers refuse others", schema.PolicyRequire{Approvers: 1, Users: []string{"carol"}}, []Decision{approval("dave", 0, nil)}, nil, false},
		{"a rejection settles it", schema.PolicyRequire{Approvers: 1}, []Decision{approval("a", 0, nil), {DeciderID: "b", At: epoch}}, nil, false},
		{"distinct teams", schema.PolicyRequire{Approvers: 2, DistinctTeams: &yes}, []Decision{approval("a", 0, nil, "t1"), approval("b", 1, nil, "t2")}, nil, true},
		{"the same team twice", schema.PolicyRequire{Approvers: 2, DistinctTeams: &yes}, []Decision{approval("a", 0, nil, "t1"), approval("b", 1, nil, "t1")}, nil, false},
		{"a team can be chosen for each approver", schema.PolicyRequire{Approvers: 2, DistinctTeams: &yes}, []Decision{approval("a", 0, nil, "t1", "t2"), approval("b", 1, nil, "t1")}, nil, true},
		{
			"parallel stages need every role",
			schema.PolicyRequire{Approvers: 2, Roles: []string{"security", "compliance"}, Stages: schema.PolicyStagesParallel},
			[]Decision{approval("a", 0, []string{"security"}), approval("b", 1, []string{"security"})},
			nil, false,
		},
		{
			"parallel stages in any order",
			schema.PolicyRequire{Approvers: 2, Roles: []string{"security", "compliance"}, Stages: schema.PolicyStagesParallel},
			[]Decision{approval("a", 0, []string{"compliance"}), approval("b", 1, []string{"security"})},
			nil, true,
		},
		{
			"sequential stages in order",
			schema.PolicyRequire{Approvers: 2, Roles: []string{"security", "compliance"}, Stages: schema.PolicyStagesSequential},
			[]Decision{approval("a", 0, []string{"security"}), approval("b", 1, []string{"compliance"})},
			nil, true,
		},
		{
			"sequential stages out of order",
			schema.PolicyRequire{Approvers: 2, Roles: []string{"security", "compliance"}, Stages: schema.PolicyStagesSequential},
			[]Decision{approval("a", 0, []string{"compliance"}), approval("b", 1, []string{"security"})},
			nil, false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Evaluate([]Policy{policyWith(c.req)}, c.decisions, c.excluded...)
			if got.Satisfied != c.want {
				t.Fatalf("Satisfied = %v (missing %v), want %v", got.Satisfied, got.Missing, c.want)
			}
		})
	}
}

// Verifies: GOV-021.
func TestEvaluate_EveryMatchingPolicyMustBeSatisfied_GOV_021(t *testing.T) {
	one, two := policyWith(schema.PolicyRequire{Approvers: 1}), policyWith(schema.PolicyRequire{Approvers: 2})
	d := []Decision{approval("a", 0, nil)}
	if out := Evaluate([]Policy{one, two}, d); out.Satisfied || out.Required != 2 {
		t.Fatalf("one approval satisfied a four-eyes policy: %+v", out)
	}
	if out := Evaluate(nil, d); out.Satisfied {
		t.Fatal("no policy cannot be satisfied; the caller must not ask")
	}
}

// Verifies: GOV-021.
func TestCanApprove_GOV_021(t *testing.T) {
	seq := policyWith(schema.PolicyRequire{Approvers: 2, Roles: []string{"security", "compliance"}, Stages: schema.PolicyStagesSequential})
	sec, comp := approval("a", 0, []string{"security"}), approval("b", 1, []string{"compliance"})
	if ok, why := CanApprove([]Policy{seq}, nil, comp); ok {
		t.Fatalf("compliance went first: %s", why)
	}
	if ok, why := CanApprove([]Policy{seq}, nil, sec); !ok {
		t.Fatalf("security should go first: %s", why)
	}
	if ok, _ := CanApprove([]Policy{seq}, []Decision{sec}, comp); !ok {
		t.Fatal("compliance should go second")
	}
	if ok, _ := CanApprove([]Policy{seq}, []Decision{sec}, sec); ok {
		t.Fatal("nobody decides twice")
	}
	if ok, _ := CanApprove([]Policy{seq}, nil, approval("x", 0, []string{"developer"})); ok {
		t.Fatal("a role the policy does not list was accepted")
	}
	open := policyWith(schema.PolicyRequire{Approvers: 1})
	if ok, _ := CanApprove([]Policy{seq, open}, nil, approval("x", 0, []string{"developer"})); !ok {
		t.Fatal("one accepting policy is enough to take the decision")
	}
}
