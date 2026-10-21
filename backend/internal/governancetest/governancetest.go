// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package governancetest builds the organisation the tests of break-glass,
// change freezes and compliance reports run in: an owner, a release manager
// who may use break-glass, a reviewer who may decide approvals and a viewer
// who may do neither, with one app and its environments, an audit log and a
// clock the test moves. It is for tests only (GOV-024 to GOV-026).
package governancetest

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// IDs generates identifiers for the services under test.
type IDs struct{ g *uuid7.Generator }

// New returns the next identifier.
func (i IDs) New() (string, error) {
	u, err := i.g.New()
	return u.String(), err
}

// World is a test organisation.
type World struct {
	DB      *storage.DB
	Log     *audit.Log
	Auth    *auth.Service
	Tenancy *tenancy.Service
	Gen     IDs
	// Now is the clock the services under test read; a test moves it.
	Now time.Time
	// Org and App identify the organisation and its app; Envs maps an
	// environment's key to its identifier.
	Org  string
	App  string
	Envs map[string]string
	// The people: the owner may do everything; the release manager holds
	// breakglass.invoke, killswitch.manage and approvals.decide; the
	// reviewer holds approvals.decide; the viewer holds neither.
	Owner    auth.Principal
	Manager  auth.Principal
	Reviewer auth.Principal
	Viewer   auth.Principal
	// Signer is the file signing backend of the world.
	Signer *signing.File
}

// Clock is the world's clock.
func (w *World) Clock() time.Time { return w.Now }

// Limits is the limits function the services take: the registry defaults.
func (*World) Limits(context.Context, pgx.Tx, string) (limits.Set, error) {
	return limits.Defaults(), nil
}

// Tx runs f in a transaction of the world's organisation.
func (w *World) Tx(ctx context.Context, f func(context.Context, pgx.Tx) error) error {
	return w.DB.InTx(ctx, storage.Tenant{OrganizationID: w.Org}, f) //nolint:wrapcheck // a test
}

// NewID returns a fresh identifier.
func (w *World) NewID(t *testing.T) string {
	t.Helper()
	id, err := w.Gen.New()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// AuditRow is the part of an audit entry the tests read.
type AuditRow struct{ TargetID, Detail string }

// Audit returns the organisation's audit entries of an action, oldest first.
func (w *World) Audit(t *testing.T, action audit.Action) []AuditRow {
	t.Helper()
	var rows []AuditRow
	err := w.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		r, err := tx.Query(ctx, `SELECT target_id, detail FROM audit_log WHERE action = $1 ORDER BY sequence`, string(action))
		if err != nil {
			return err //nolint:wrapcheck // a test
		}
		defer r.Close()
		for r.Next() {
			var row AuditRow
			if err := r.Scan(&row.TargetID, &row.Detail); err != nil {
				return err //nolint:wrapcheck // a test
			}
			rows = append(rows, row)
		}
		return r.Err() //nolint:wrapcheck // a test
	})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// Licensed returns ctx with a valid licence that lists the features.
func Licensed(ctx context.Context, features ...edition.Feature) context.Context {
	return edition.WithProvider(ctx, licence{status: edition.Status{State: edition.StateValid, Features: features}})
}

// licence is an edition provider with a fixed status.
type licence struct{ status edition.Status }

// Status returns the fixed status.
func (l licence) Status() edition.Status { return l.status }

// New builds a world on the test database.
func New(t *testing.T) *World {
	t.Helper()
	ctx := context.Background()
	db := storagetest.Open(t)
	signer, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gen := IDs{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	log := audit.NewLog(gen, nil)
	w := &World{DB: db, Log: log, Gen: gen, Now: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC), Envs: map[string]string{}, Signer: signer}
	if w.Auth, err = auth.NewService(auth.Options{DB: db, Audit: log, Cache: cache.NewMemory(nil), Crypter: signer, IDs: gen, VerificationURI: "https://p.example/device"}); err != nil {
		t.Fatal(err)
	}
	if w.Tenancy, err = tenancy.NewService(tenancy.Options{DB: db, Audit: log, Auth: w.Auth, Crypter: signer, IDs: gen, SigningKeyPrefix: "targets", Now: w.Clock}); err != nil {
		t.Fatal(err)
	}
	w.populate(ctx, t)
	return w
}

// populate creates the organisation, the app and the people.
func (w *World) populate(ctx context.Context, t *testing.T) {
	t.Helper()
	admin, invitation, err := w.Auth.Bootstrap(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Auth.AcceptInvitation(ctx, invitation, "Admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	adminID := auth.Identity{Kind: auth.KindUser, ID: admin.ID, UserID: admin.ID, Display: "Admin", SecondFactor: true, InstallationAdmin: true}.WithStepUp(0)
	o, err := w.Tenancy.CreateOrganization(ctx, adminID, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	w.Org = o.ID
	if w.Owner, err = w.Auth.Resolve(ctx, adminID, w.Org); err != nil {
		t.Fatal(err)
	}
	app, err := w.Tenancy.CreateApp(ctx, w.Owner, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	w.App = app.ID
	envs, err := w.Tenancy.ListEnvironments(ctx, w.Owner, w.App, tenancy.Page{Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range envs {
		w.Envs[e.Key] = e.ID
	}
	w.Manager = w.member(ctx, t, "manager@example.com", "release-manager")
	w.Reviewer = w.member(ctx, t, "reviewer@example.com", "reviewer")
	w.Viewer = w.member(ctx, t, "viewer@example.com", "viewer")
}

// member adds a person with a role and returns them signed in with a fresh
// step-up.
func (w *World) member(ctx context.Context, t *testing.T, email, role string) auth.Principal {
	t.Helper()
	m, _, err := w.Tenancy.AddMember(ctx, w.Owner, "", email, role)
	if err != nil {
		t.Fatal(err)
	}
	id := auth.Identity{Kind: auth.KindUser, ID: m.UserID, UserID: m.UserID, Display: email, SecondFactor: true}.WithStepUp(0)
	p, err := w.Auth.Resolve(ctx, id, w.Org)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
