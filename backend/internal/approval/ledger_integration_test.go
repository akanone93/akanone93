// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/prodchange"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// guard is a change-freeze guard the test switches on and off.
type guard struct {
	held  bool
	asked []string
}

// Check implements freeze.Guard.
func (g *guard) Check(_ context.Context, _ pgx.Tx, environmentKey string) error {
	g.asked = append(g.asked, environmentKey)
	if g.held {
		return plxerr.New(plxerr.ChangeFreezeActive, "frozen")
	}
	return nil
}

// ledger reads the production changes around the fixture's clock.
func (f *fixture) ledger(t *testing.T) []dbgen.ProductionChange {
	t.Helper()
	var rows []dbgen.ProductionChange
	err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = prodchange.Between(ctx, tx, f.now.Add(-time.Hour), f.now.Add(time.Hour), 100)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// limitRaise is a change no default policy guards, in production.
func (f *fixture) limitRaise(n int) approval.Artifact {
	return approval.Artifact{
		Resource: schema.PolicyResourceLimit, OrganizationID: f.org, AppID: f.app, Action: approval.ActionRaiseLimit,
		Environment: approval.Environment{ID: f.envs["production"], Key: "production", Production: true},
		Subject:     "limit:a11y.waiversPerPlugin", Hash: approval.HashOf([]byte(strconv.Itoa(n))), Summary: "raise a limit", Requester: f.owner.Actor(),
	}
}

// Verifies: GOV-025.
// A change to a production environment inside a freeze is refused with
// PLX-8010 before a request is opened; a change to another environment is not
// held, and outside the freeze the engine decides as before.
func TestRequire_ChangeFreeze_GOV_025(t *testing.T) {
	f := newFixture(t)
	g := &guard{held: true}
	f.svc.UseFreezes(g)
	wantCode(t, f.require(t, f.release("production", 1, "a")), plxerr.ChangeFreezeActive, "production inside a freeze")
	if list, err := f.svc.ListApprovals(context.Background(), f.owner, f.app, "", storage.Cursor{}, 10); err != nil || len(list) != 0 {
		t.Fatalf("a refused change opened requests: %+v, %v", list, err)
	}
	if err := f.require(t, f.release("development", 1, "a")); err != nil {
		t.Fatalf("development inside a freeze: %v", err)
	}
	if len(g.asked) != 1 || g.asked[0] != "production" {
		t.Errorf("the guard was asked about %v, want production once", g.asked)
	}
	g.held = false
	wantCode(t, f.require(t, f.release("production", 1, "a")), plxerr.ApprovalRequired, "production outside the freeze")
	if len(f.ledger(t)) != 0 {
		t.Error("a refused change was entered in the ledger")
	}
}

// Verifies: GOV-026.
// Every production change that goes through the engine is entered in the
// ledger, with the request that authorised it when there was one, and in the
// same transaction: a refused change leaves no row and a development change
// none either.
func TestRequire_LedgerRecordsProductionChanges_GOV_026(t *testing.T) {
	f := newFixture(t)
	f.now = time.Now()

	if err := f.require(t, f.limitRaise(1)); err != nil {
		t.Fatal(err)
	}
	art := f.release("production", 7, "payload")
	art.Author = f.reviewer.UserID
	id := requestOf(t, f.require(t, art))
	if _, err := f.decide(f.second, id, approval.DecisionApprove); err != nil {
		t.Fatal(err)
	}
	if err := f.require(t, art); err != nil {
		t.Fatal(err)
	}
	if err := f.require(t, f.release("development", 8, "payload")); err != nil {
		t.Fatal(err)
	}

	rows := f.ledger(t)
	if len(rows) != 2 {
		t.Fatalf("%d ledger rows, want the two production changes: %+v", len(rows), rows)
	}
	unguarded, approved := rows[0], rows[1]
	if unguarded.Resource != "limit" || unguarded.RequestID.Valid || unguarded.ChangedByID != f.owner.Actor().ID || unguarded.EnvironmentKey != "production" {
		t.Errorf("the unguarded change = %+v", unguarded)
	}
	if approved.Resource != "release" || storage.ID(approved.RequestID) != id || approved.AuthorID != f.reviewer.UserID ||
		approved.ContentHash != art.Hash || approved.Subject != "release:7" || storage.ID(approved.AppID) != f.app {
		t.Errorf("the approved change = %+v, request %s", approved, id)
	}
}
