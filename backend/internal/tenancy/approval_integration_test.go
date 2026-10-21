// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package tenancy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// Verifies: LIM-006, LIM-002, GOV-020.
// An organisation may tighten a limit freely and raise it again up to the
// installation's value; raising it is a change the approval engine guards
// when a policy of the organisation says so, and an approval is spent by
// the raise it approves. Raising past the installation's value stays
// refused.
func TestRaiseOrganizationLimitNeedsApproval_LIM_006(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	engine, err := approval.New(approval.Options{DB: f.db, Audit: f.log, IDs: f.ids})
	if err != nil {
		t.Fatal(err)
	}
	guarded, err := tenancy.NewService(tenancy.Options{
		DB: f.db, Audit: f.log, Auth: f.auth, Crypter: f.crypto, IDs: f.ids, SigningKeyPrefix: "targets", Approvals: engine,
		Now: func() time.Time { return f.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := f.svc.AddMember(ctx, f.owner, "", "r@example.com", "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := f.auth.Resolve(ctx, auth.Identity{Kind: auth.KindUser, ID: m.UserID, UserID: m.UserID, Display: "r", SecondFactor: true}.WithStepUp(0), f.org)
	if err != nil {
		t.Fatal(err)
	}
	key := string(limits.AppPlugins)
	installation := limits.Defaults().Get(limits.AppPlugins)

	if _, err := guarded.SetOrganizationLimit(ctx, f.owner, key, 5); err != nil {
		t.Fatalf("tightening asks nothing: %v", err)
	}
	if _, err := guarded.SetOrganizationLimit(ctx, f.owner, key, 8); err != nil {
		t.Fatalf("raising with no policy asks nothing: %v", err)
	}
	policy := `{"schemaVersion":"1.0.0","kind":"policy","id":"01b0c450-6c00-7000-8000-0000000000a1","key":"raise-limits","name":"Raise limits","match":{"resources":["limit"]},"require":{"approvers":1}}`
	if _, err := engine.PutPolicy(ctx, f.owner, "", []byte(policy)); err != nil {
		t.Fatal(err)
	}
	if _, err := guarded.SetOrganizationLimit(ctx, f.owner, key, 3); err != nil {
		t.Fatalf("tightening still asks nothing: %v", err)
	}
	_, err = guarded.SetOrganizationLimit(ctx, f.owner, key, 7)
	var refusal *plxerr.Error
	if !errors.As(err, &refusal) || refusal.Code != plxerr.ApprovalRequired {
		t.Fatalf("raising under a policy: %v", err)
	}
	if _, err := engine.Decide(ctx, reviewer, "", refusal.Details["request"], approval.DecisionApprove, "fine"); err != nil {
		t.Fatalf("approving: %v", err)
	}
	if u, err := guarded.SetOrganizationLimit(ctx, f.owner, key, 7); err != nil || u.Effective != 7 {
		t.Fatalf("the approved raise: %+v, %v", u, err)
	}
	if _, err := guarded.SetOrganizationLimit(ctx, f.owner, key, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := guarded.SetOrganizationLimit(ctx, f.owner, key, 7); !errors.As(err, &refusal) || refusal.Code != plxerr.ApprovalRequired {
		t.Fatalf("the approval was spent: %v", err)
	}
	if _, err := guarded.SetOrganizationLimit(ctx, f.owner, key, installation+1); !errors.As(err, &refusal) || refusal.Code != plxerr.OutOfRange {
		t.Fatalf("raising past the installation's limit: %v", err)
	}
}
