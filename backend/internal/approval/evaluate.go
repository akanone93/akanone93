// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/schema"
)

// Decision is what the evaluation reads of a recorded decision: who decided,
// what they held at the time, and when.
type Decision struct {
	// DeciderID is the user who decided.
	DeciderID string
	// Roles are the role keys the decider held when they decided.
	Roles []string
	// Teams are the teams the decider belonged to when they decided.
	Teams []string
	// Approve is true for an approval and false for a rejection.
	Approve bool
	// At is when the decision was made.
	At time.Time
}

// Outcome is where a request stands against the policies that match it.
type Outcome struct {
	// Satisfied is true when every matching policy has the approvals it
	// asks for.
	Satisfied bool
	// Rejected is true when anyone rejected the request.
	Rejected bool
	// Missing says, for each policy not yet satisfied, what it still lacks.
	Missing []string
	// Required is the most approvals any matching policy asks for.
	Required int
}

// Evaluate decides whether the decisions on a request satisfy every policy
// that matches it. The decisions of the people in excluded (the requester
// and the author of the change) never count (PLX-8003), so no policy can be
// satisfied by the person who made the change. The decisions must all be
// about the request's current content: that is GOV-022's guarantee, kept by
// binding a request to one hash and never reusing its decisions for another.
func Evaluate(policies []Policy, decisions []Decision, excluded ...string) Outcome {
	out := Outcome{Satisfied: len(policies) > 0}
	var approvals []Decision
	for _, d := range decisions {
		switch {
		case slices.Contains(excluded, d.DeciderID):
		case !d.Approve:
			out.Rejected = true
		default:
			approvals = append(approvals, d)
		}
	}
	slices.SortStableFunc(approvals, func(a, b Decision) int { return a.At.Compare(b.At) })
	for _, p := range policies {
		out.Required = max(out.Required, int(p.Doc.Require.Approvers))
		if missing := unmet(p, approvals); missing != "" {
			out.Satisfied = false
			out.Missing = append(out.Missing, missing)
		}
	}
	out.Satisfied = out.Satisfied && !out.Rejected
	return out
}

// unmet says what a policy still lacks, or "" when it is satisfied.
// approvals are in the order they were given.
func unmet(p Policy, approvals []Decision) string {
	req := p.Doc.Require
	eligible := slices.DeleteFunc(slices.Clone(approvals), func(d Decision) bool { return !eligibleFor(req, d) })
	slots := slotsOf(req)
	need := int(req.Approvers)
	if req.Stages != "" {
		need = max(need, len(slots))
	}
	if len(eligible) < need {
		return fmt.Sprintf("%s needs %d approval(s), %d given", p.Doc.Key, need, len(eligible))
	}
	if req.DistinctTeams != nil && *req.DistinctTeams && distinctTeamCount(eligible) < need {
		return fmt.Sprintf("%s needs approvals from %d different teams", p.Doc.Key, need)
	}
	if req.Stages != "" && !stagesMet(req.Stages, slots, eligible) {
		return fmt.Sprintf("%s needs every listed approver to approve, %s", p.Doc.Key, stagesWord(req.Stages))
	}
	return ""
}

// stagesWord describes a staging for a message.
func stagesWord(s schema.PolicyStages) string {
	if s == schema.PolicyStagesSequential {
		return "one after the other in the order listed"
	}
	return "all of them"
}

// slot is one role or named approver a staged policy lists.
type slot struct {
	role string
	user string
}

// slotsOf lists the roles and then the named approvers of a requirement:
// the stages of a sequential policy, and the places a parallel policy fills.
func slotsOf(req schema.PolicyRequire) []slot {
	out := make([]slot, 0, len(req.Roles)+len(req.Users))
	for _, r := range req.Roles {
		out = append(out, slot{role: r})
	}
	for _, u := range req.Users {
		out = append(out, slot{user: u})
	}
	return out
}

// fits reports whether a decider can fill a slot.
func (s slot) fits(d Decision) bool {
	if s.role != "" {
		return slices.Contains(d.Roles, s.role)
	}
	return d.DeciderID == s.user
}

// eligibleFor reports whether the decider is one a requirement accepts: a
// member of one of its roles or one of its named approvers, or anyone when
// it names neither.
func eligibleFor(req schema.PolicyRequire, d Decision) bool {
	slots := slotsOf(req)
	if len(slots) == 0 {
		return true
	}
	return slices.ContainsFunc(slots, func(s slot) bool { return s.fits(d) })
}

// stagesMet reports whether the approvals fill every slot with a different
// approver and, for a sequential policy, in the order the slots are listed.
func stagesMet(stages schema.PolicyStages, slots []slot, approvals []Decision) bool {
	if stages == schema.PolicyStagesSequential {
		for i, s := range slots {
			if i >= len(approvals) || !s.fits(approvals[i]) {
				return false
			}
		}
		return true
	}
	return matchSize(len(slots), len(approvals), func(i, j int) bool { return slots[i].fits(approvals[j]) }) == len(slots)
}

// distinctTeamCount is the number of approvals that can be given a team of
// their own, no two the same.
func distinctTeamCount(approvals []Decision) int {
	var teams []string
	for _, a := range approvals {
		for _, t := range a.Teams {
			if !slices.Contains(teams, t) {
				teams = append(teams, t)
			}
		}
	}
	return matchSize(len(approvals), len(teams), func(i, j int) bool { return slices.Contains(approvals[i].Teams, teams[j]) })
}

// matchSize returns the size of a largest set of pairs (i, j), no left and
// no right item used twice, among n left and m right items; edge says
// whether a pair is allowed. It is the augmenting-path algorithm; the sets
// here have at most a few dozen items.
func matchSize(n, m int, edge func(i, j int) bool) int {
	owner := make([]int, m)
	for j := range owner {
		owner[j] = -1
	}
	var try func(i int, seen []bool) bool
	try = func(i int, seen []bool) bool {
		for j := range m {
			if !edge(i, j) || seen[j] {
				continue
			}
			seen[j] = true
			if owner[j] < 0 || try(owner[j], seen) {
				owner[j] = i
				return true
			}
		}
		return false
	}
	size := 0
	for i := range n {
		if try(i, make([]bool, m)) {
			size++
		}
	}
	return size
}

// CanApprove reports whether the decider may add an approval now: a policy
// accepts them, they have not approved already, and, where the policy is
// sequential, it is their turn. reason says why not.
func CanApprove(policies []Policy, approvals []Decision, d Decision) (ok bool, reason string) {
	if slices.ContainsFunc(approvals, func(a Decision) bool { return a.DeciderID == d.DeciderID }) {
		return false, "you have already decided this request"
	}
	var reasons []string
	for _, p := range policies {
		why := acceptance(p.Doc, approvals, d)
		if why == "" {
			return true, ""
		}
		reasons = append(reasons, why)
	}
	return false, strings.Join(reasons, "; ")
}

// acceptance says why a policy does not accept the decider now, or "".
func acceptance(doc schema.PolicyDocument, approvals []Decision, d Decision) string {
	req := doc.Require
	if !eligibleFor(req, d) {
		return fmt.Sprintf("%s accepts only its listed roles and approvers", doc.Key)
	}
	if req.Stages != schema.PolicyStagesSequential {
		return ""
	}
	slots := slotsOf(req)
	turn := len(approvals)
	if turn < len(slots) && !slots[turn].fits(d) {
		return fmt.Sprintf("%s takes its approvers one after the other, and it is not your turn", doc.Key)
	}
	return ""
}
