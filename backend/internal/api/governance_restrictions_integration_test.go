// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// Verifies: GOV-006, GOV-007.
// Across the edge, a restriction made through the API takes a permission from
// its person on one app, which the effective set and the explanation both
// say, names the restriction in the explanation, and is lifted by deleting it.
func TestRestrictionsAtTheEdge_GOV_007(t *testing.T) {
	f := newMatrixFixture(t)
	ctx := context.Background()
	bob, bobID := f.scopedMember(t)
	must(f.w.govern.CreateGrant(ctx, req(f.owner(), &pluxv1.CreateGrantRequest{UserId: bobID, Role: "developer", Scope: "organization"})))(t)
	effective := func(appID string) *pluxv1.GetEffectivePermissionsResponse {
		return must(f.w.govern.GetEffectivePermissions(ctx, req(bob, &pluxv1.GetEffectivePermissionsRequest{AppId: appID})))(t)
	}
	if got := effective(f.app); !slices.Contains(got.GetPermissions(), "plugin.edit") || got.GetVersion() == 0 {
		t.Fatalf("effective permissions before the restriction = %v", got)
	}

	made := must(f.w.govern.CreateRestriction(ctx, req(f.owner(), &pluxv1.CreateRestrictionRequest{
		SubjectKind: "user", SubjectId: bobID, Patterns: []string{"plugin.edit"}, AppId: f.app, Reason: "freeze the shop",
	})))(t).GetRestriction()
	if made.GetId() == "" || made.GetReason() != "freeze the shop" || made.GetAppId() != f.app {
		t.Errorf("restriction = %v", made)
	}
	listed := must(f.w.govern.ListRestrictions(ctx, req(f.owner(), &pluxv1.ListRestrictionsRequest{SubjectId: bobID})))(t).GetRestrictions()
	if len(listed) != 1 || listed[0].GetId() != made.GetId() {
		t.Errorf("ListRestrictions = %v", listed)
	}

	after := effective(f.app)
	if slices.Contains(after.GetPermissions(), "plugin.edit") || !slices.Contains(after.GetPermissions(), "app.read") {
		t.Errorf("effective permissions on the restricted app = %v", after.GetPermissions())
	}
	if !slices.Contains(effective(f.freshApp(t)).GetPermissions(), "plugin.edit") {
		t.Error("the restriction on one app reached another")
	}
	if !slices.IsSorted(after.GetPermissions()) {
		t.Errorf("the set is not in name order: %v", after.GetPermissions())
	}

	explain := func(c caller, m *pluxv1.ExplainAccessRequest) (*pluxv1.ExplainAccessResponse, error) {
		r, err := f.w.govern.ExplainAccess(ctx, req(c, m))
		if err != nil {
			return nil, err //nolint:wrapcheck // the code is what the test reads
		}
		return r.Msg, nil
	}
	own, err := explain(bob, &pluxv1.ExplainAccessRequest{Permission: "plugin.edit", AppId: f.app})
	if err != nil || own.GetDecision() != "deny" || !slices.Equal(own.GetRemovingRestrictionIds(), []string{made.GetId()}) || len(own.GetAllowingGrantIds()) == 0 {
		t.Errorf("Bob explaining his own plugin.edit = %v, %v", own, err)
	}
	if _, err := explain(bob, &pluxv1.ExplainAccessRequest{UserId: bobID, Permission: "plugin.edit", AppId: f.app}); err != nil {
		t.Errorf("Bob naming himself: %v", err)
	}
	if _, err := explain(bob, &pluxv1.ExplainAccessRequest{UserId: "018f0000-0000-7000-8000-000000000000", Permission: "app.read"}); codeOf(err) != connect.CodePermissionDenied {
		t.Errorf("Bob explaining someone else: %v, want PermissionDenied", err)
	}
	proposal := &pluxv1.ExplainAccessRequest{UserId: bobID, Permission: "plugin.edit", AppId: f.app}
	proposal.Proposal = &pluxv1.ExplainAccessRequest_ProposedGrant{ProposedGrant: &pluxv1.ProposedGrant{UserId: bobID, Role: "owner", Scope: "organization"}}
	if got, err := explain(bob, proposal); err != nil || got.GetDecision() != "deny" {
		t.Errorf("Bob simulating a grant for himself = %v, %v; his own access is his to explain", got, err)
	}
	if got, err := explain(f.owner(), proposal); err != nil || got.GetDecision() != "deny" || !slices.Contains(got.GetAllowingGrantIds(), "proposed") {
		t.Errorf("the owner simulating Bob as an owner = %v, %v; the restriction still wins", got, err)
	}
	if saved := must(f.w.govern.ListRestrictions(ctx, req(f.owner(), &pluxv1.ListRestrictionsRequest{})))(t).GetRestrictions(); len(saved) != 1 {
		t.Errorf("%d restrictions after simulating, want the one made", len(saved))
	}

	if _, err := f.w.govern.CreateRestriction(ctx, req(bob, &pluxv1.CreateRestrictionRequest{SubjectKind: "user", SubjectId: bobID, Patterns: []string{"app.read"}})); codeOf(err) != connect.CodePermissionDenied {
		t.Errorf("Bob restricting: %v, want PermissionDenied", err)
	}
	must(f.w.govern.DeleteRestriction(ctx, req(f.owner(), &pluxv1.DeleteRestrictionRequest{Id: made.GetId()})))(t)
	if !slices.Contains(effective(f.app).GetPermissions(), "plugin.edit") {
		t.Error("deleting the restriction did not give plugin.edit back")
	}
}

// Verifies: GOV-006.
// A person who may list every app is not shown an app on which a restriction
// takes app.read away.
func TestListAppsLeavesOutRestrictedApps_GOV_006(t *testing.T) {
	f := newMatrixFixture(t)
	ctx := context.Background()
	bob, bobID := f.scopedMember(t)
	other := f.freshApp(t)
	keys := func() []string {
		var ids []string
		for _, a := range must(f.w.app.ListApps(ctx, req(bob, &pluxv1.ListAppsRequest{})))(t).GetApps() {
			ids = append(ids, a.GetId())
		}
		return ids
	}
	if got := keys(); !slices.Contains(got, f.app) || !slices.Contains(got, other) {
		t.Fatalf("a viewer lists %v, want both apps", got)
	}
	must(f.w.govern.CreateRestriction(ctx, req(f.owner(), &pluxv1.CreateRestrictionRequest{
		SubjectKind: "user", SubjectId: bobID, Patterns: []string{"app.read"}, AppId: f.app,
	})))(t)
	if got := keys(); slices.Contains(got, f.app) || !slices.Contains(got, other) {
		t.Errorf("with app.read removed on the shop a viewer lists %v", got)
	}
}

// Verifies: GOV-007.
// WatchPermissions sends the organisation's version first and a newer one
// after a change another caller makes.
func TestWatchPermissionsAtTheEdge_GOV_007(t *testing.T) {
	f := newMatrixFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	bob, bobID := f.scopedMember(t)
	stream, err := f.w.govern.WatchPermissions(ctx, req(bob, &pluxv1.WatchPermissionsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stream.Close() }()
	if !stream.Receive() {
		t.Fatalf("no first version: %v", stream.Err())
	}
	first := stream.Msg()
	if first.GetOrganizationId() != f.org || first.GetVersion() == 0 {
		t.Fatalf("first message = %v", first)
	}
	must(f.w.govern.CreateGrant(ctx, req(f.owner(), &pluxv1.CreateGrantRequest{UserId: bobID, Role: "developer", Scope: "organization"})))(t)
	if !stream.Receive() {
		t.Fatalf("no version after a change: %v", stream.Err())
	}
	if got := stream.Msg().GetVersion(); got <= first.GetVersion() {
		t.Errorf("version after the change = %d, not newer than %d", got, first.GetVersion())
	}
}

// Verifies: GOV-007, SRV-065.
// Explain calls are limited per person, and the calls that simulate a
// proposal by a tighter allowance of their own: spending one does not spend
// the other.
func TestExplainIsRateLimited_GOV_007(t *testing.T) {
	f := newMatrixFixture(t)
	ctx := context.Background()
	owner := f.owner()
	simulations, explanations := limits.Defaults().Get(limits.AuthSimulatePerMinute), limits.Defaults().Get(limits.AuthExplainPerMinute)
	explain := func(m *pluxv1.ExplainAccessRequest) connect.Code {
		_, err := f.w.govern.ExplainAccess(ctx, req(owner, m))
		return codeOf(err)
	}
	plain := &pluxv1.ExplainAccessRequest{Permission: "app.read"}
	simulated := &pluxv1.ExplainAccessRequest{Permission: "app.read"}
	simulated.Proposal = &pluxv1.ExplainAccessRequest_ProposedRestriction{ProposedRestriction: &pluxv1.CreateRestrictionRequest{
		SubjectKind: "organization", Patterns: []string{"app.read"},
	}}

	for i := int64(0); i < simulations; i++ {
		if c := explain(simulated); c != 0 {
			t.Fatalf("simulation %d of %d allowed: %v", i+1, simulations, c)
		}
	}
	if c := explain(simulated); c != connect.CodeResourceExhausted {
		t.Errorf("simulation %d = %v, want ResourceExhausted", simulations+1, c)
	}
	if c := explain(plain); c != 0 {
		t.Errorf("an explanation after the simulations were spent = %v; it has its own allowance", c)
	}
	for i := int64(1); i < explanations; i++ {
		if c := explain(plain); c != 0 {
			t.Fatalf("explanation %d of %d allowed: %v", i+1, explanations, c)
		}
	}
	if c := explain(plain); c != connect.CodeResourceExhausted {
		t.Errorf("explanation %d = %v, want ResourceExhausted", explanations+1, c)
	}
}
