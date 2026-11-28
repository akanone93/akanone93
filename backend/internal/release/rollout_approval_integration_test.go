// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// prepareRollout asks to roll a release out on an environment's default
// channel, as the rollout service does, in a transaction of its own.
func (f *fixture) prepareRollout(p auth.Principal, envID string, candidate int64) error {
	return f.db.InTx(context.Background(), storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := f.rel.PrepareRollout(ctx, tx, p, f.app, envID, "", candidate)
		return err
	})
}

// releaseManager adds a Release Manager who signed in with a second factor
// but has made no step-up.
func (f *fixture) releaseManager(t *testing.T, email string) auth.Principal {
	t.Helper()
	ctx := context.Background()
	m, _, err := f.tenancy.AddMember(ctx, f.owner, "", email, "release-manager")
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.auth.Resolve(ctx, auth.Identity{Kind: auth.KindUser, ID: m.UserID, UserID: m.UserID, Display: email, SecondFactor: true}, f.org)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Verifies: GOV-020, REL-050, SEC-103.
// A rollout serves a release to a production channel's devices as a promotion
// does, so it needs what a promotion needs (build log row 321): the step-up
// of release.promote, and the approval the channel's policy asks for, spent
// by the rollout. The server completing the rollout then promotes the
// candidate without being asked again.
func TestRolloutNeedsWhatAPromotionNeeds_GOV_020(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g := newGuarded(t, settings.Standard)
	reviewer := g.reviewer(t, "r@example.com")
	prod := g.envs["production"].ID

	// Release 1 is on Production, approved.
	first := g.releaseOfDevelopment(t)
	_, err := g.rel.PromoteRelease(ctx, g.owner, g.app, first.Sequence, prod, "")
	if _, err := g.engine.Decide(ctx, reviewer, g.app, requestNamed(t, err), approval.DecisionApprove, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := g.rel.PromoteRelease(ctx, g.owner, g.app, first.Sequence, prod, ""); err != nil {
		t.Fatalf("the approved promotion of release 1: %v", err)
	}
	// Release 2 is made in development.
	g.editResult(t, "second")
	second, diags, err := g.rel.CreateRelease(ctx, g.owner, g.app, g.envs["development"].ID, nil, "")
	if err != nil || diags.HasErrors() {
		t.Fatalf("CreateRelease: %v %v", diags, err)
	}

	if err := g.prepareRollout(g.releaseManager(t, "rm@example.com"), prod, second.Sequence); code(err) != plxerr.StepUpRequired {
		t.Fatalf("a rollout without a step-up: %v, want PLX-6200", err)
	}
	id := requestNamed(t, g.prepareRollout(g.owner, prod, second.Sequence))
	if _, err := g.engine.Decide(ctx, reviewer, g.app, id, approval.DecisionApprove, "rollout reviewed"); err != nil {
		t.Fatal(err)
	}
	if err := g.prepareRollout(g.owner, prod, second.Sequence); err != nil {
		t.Fatalf("the approved rollout: %v", err)
	}
	// The approval is spent: a second rollout of the same release asks again.
	requestNamed(t, g.prepareRollout(g.owner, prod, second.Sequence))

	// The server completing the rollout promotes the candidate unasked.
	r, err := g.rel.PromoteRelease(ctx, auth.System(g.org), g.app, second.Sequence, prod, "")
	if err != nil || r.Sequence != second.Sequence {
		t.Fatalf("the server's promotion at the rollout's end: %+v %v", r, err)
	}
	// A person promoting is still asked.
	_, err = g.rel.PromoteRelease(ctx, g.owner, g.app, first.Sequence, prod, "")
	requestNamed(t, err)
}
