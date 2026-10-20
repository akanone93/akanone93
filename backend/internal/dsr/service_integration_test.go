// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package dsr_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/dsr"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
)

// ids adapts the UUID generator to the identifier interface of the service.
type ids struct{ g *uuid7.Generator }

func (i ids) New() (string, error) {
	u, err := i.g.New()
	if err != nil {
		return "", err //nolint:wrapcheck // a test
	}
	return u.String(), nil
}

// tenant is an organisation with an app, an environment and an administrator.
type tenant struct {
	org, app, env string
	admin         auth.Principal
}

// world is a database with two organisations and a service whose export
// limit a test can change.
type world struct {
	t      *testing.T
	db     *storage.DB
	svc    *dsr.Service
	ids    ids
	budget int64
	a, b   tenant
}

func newWorld(t *testing.T) *world {
	t.Helper()
	g := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	w := &world{t: t, db: storagetest.Open(t), ids: g, budget: limits.Defaults().Get(limits.DsrExportBytes)}
	svc, err := dsr.NewService(dsr.Options{
		DB: w.db, Audit: audit.NewLog(g, nil), IDs: g,
		Limits: func(context.Context, pgx.Tx, string) (limits.Set, error) {
			return limits.Defaults().Tighten(limits.DsrExportBytes, limits.ScopeOrganization, w.budget)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	w.svc = svc
	w.a, w.b = w.tenant("acme"), w.tenant("globex")
	return w
}

func (w *world) id() string {
	w.t.Helper()
	s, err := w.ids.New()
	if err != nil {
		w.t.Fatal(err)
	}
	return s
}

func (w *world) exec(org, sql string, args ...any) {
	w.t.Helper()
	err := w.db.InTx(context.Background(), storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err //nolint:wrapcheck // a test
	})
	if err != nil {
		w.t.Fatalf("%s: %v", sql, err)
	}
}

// tenant creates an organisation with an app and an environment.
func (w *world) tenant(key string) tenant {
	w.t.Helper()
	x := tenant{org: w.id(), app: w.id(), env: w.id()}
	if _, err := w.db.Pool().Exec(context.Background(), `INSERT INTO organizations (id, key, name) VALUES ($1, $2, $2)`, storage.MustUUID(x.org), key); err != nil {
		w.t.Fatal(err)
	}
	w.exec(x.org, `INSERT INTO apps (id, organization_id, key, name) VALUES ($1, $2, 'demo', 'Demo')`, storage.MustUUID(x.app), storage.MustUUID(x.org))
	w.exec(x.org, `INSERT INTO environments (id, organization_id, app_id, key, name, production) VALUES ($1, $2, $3, 'production', 'Production', true)`,
		storage.MustUUID(x.env), storage.MustUUID(x.org), storage.MustUUID(x.app))
	x.admin = auth.Principal{
		Identity:       auth.Identity{Kind: auth.KindSystem, ID: "dsr-test", SecondFactor: true},
		OrganizationID: x.org,
		Permissions:    []auth.Permission{auth.DSRManage},
	}
	return x
}

// seedDevice puts a row into every store keyed by a device and returns its ID.
func (w *world) seedDevice(x tenant) string {
	w.t.Helper()
	d := w.id()
	o, a, e, dev := storage.MustUUID(x.org), storage.MustUUID(x.app), storage.MustUUID(x.env), storage.MustUUID(d)
	w.exec(x.org, `INSERT INTO devices (id, organization_id, app_id, environment_id, platform, secret_hash) VALUES ($1, $2, $3, $4, 'android', '\x00')`, dev, o, a, e)
	w.exec(x.org, `INSERT INTO device_tokens (id, organization_id, device_id, secret_hash, expires_at) VALUES ($1, $2, $3, $4, now() + interval '1 hour')`,
		storage.MustUUID(w.id()), o, dev, []byte(d))
	w.exec(x.org, `INSERT INTO device_bundles (device_id, organization_id, plugin_key, bundle_sha256) VALUES ($1, $2, 'home', '\x01')`, dev, o)
	group := storage.MustUUID(w.id())
	w.exec(x.org, `INSERT INTO error_groups (id, organization_id, app_id, release_sequence, fingerprint, code, plugin_key, route, node_path, frames, fatal, first_seen, last_seen)
		VALUES ($1, $2, $3, 1, $4, 'PLX-4020', 'home', '/', '/', '', false, now(), now())`, group, o, a, d)
	w.exec(x.org, `INSERT INTO error_group_devices (group_id, organization_id, device_id) VALUES ($1, $2, $3)`, group, o, dev)
	w.exec(x.org, `INSERT INTO device_sca_keys (device_id, organization_id, public_key, jkt, key_storage, attestation_provider) VALUES ($1, $2, '\x02', 'jkt', 'hardware', 'none')`, dev, o)
	w.exec(x.org, `INSERT INTO device_push_tokens (device_id, organization_id, app_id, environment_id, provider, ciphertext, wrapped_key, key_id) VALUES ($1, $2, $3, $4, 'fcm', '\x03', '\x04', 'k1')`, dev, o, a, e)
	w.exec(x.org, `INSERT INTO telemetry_events (id, organization_id, app_id, environment_id, device_id, name, time) VALUES ($1, $2, $3, $4, $5, 'open', now())`,
		storage.MustUUID(w.id()), o, a, e, dev)
	w.exec(x.org, `INSERT INTO analytics_events (id, organization_id, app_id, environment_id, device_id, name, time) VALUES ($1, $2, $3, $4, $5, 'open', now())`,
		storage.MustUUID(w.id()), o, a, e, dev)
	return d
}

// seedUser puts a row into every store keyed by a pseudonymous user.
func (w *world) seedUser(x tenant) string {
	w.t.Helper()
	u := w.id()
	o, user := storage.MustUUID(x.org), storage.MustUUID(u)
	if _, err := w.db.Pool().Exec(context.Background(), `INSERT INTO users (id, email, display_name) VALUES ($1, $2, 'Pat')`, user, u+"@example.test"); err != nil {
		w.t.Fatal(err)
	}
	w.exec(x.org, `INSERT INTO notifications (id, organization_id, user_id, event_id, event_type, title, body) VALUES ($1, $2, $3, $4, 'release.published', 't', 'b')`,
		storage.MustUUID(w.id()), o, user, storage.MustUUID(w.id()))
	w.exec(x.org, `INSERT INTO notification_subscriptions (id, organization_id, subject_kind, subject_id, event_type) VALUES ($1, $2, 'user', $3, 'release.published')`,
		storage.MustUUID(w.id()), o, user)
	return u
}

// count returns the rows of the organisation's table whose column equals id.
func (w *world) count(x tenant, table, column, id string) int {
	w.t.Helper()
	var n int
	err := w.db.InTx(context.Background(), storage.Tenant{OrganizationID: x.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE "+column+" = $1", storage.MustUUID(id)).Scan(&n) //nolint:gosec // fixed test identifiers
	})
	if err != nil {
		w.t.Fatal(err)
	}
	return n
}

// deviceTables are the stores a device is erased from, with their key column.
var deviceTables = map[string]string{
	"telemetry_events": "device_id", "analytics_events": "device_id", "device_push_tokens": "device_id",
	"device_sca_keys": "device_id", "device_tokens": "device_id", "device_bundles": "device_id", "error_group_devices": "device_id", "devices": "id",
}

// userTables are the stores a user is erased from, with their key column.
var userTables = map[string]string{"notifications": "user_id", "notification_subscriptions": "subject_id"}

func (w *world) assertRows(x tenant, tables map[string]string, id string, want int) {
	w.t.Helper()
	for table, column := range tables {
		if got := w.count(x, table, column, id); got != want {
			w.t.Errorf("%s holds %d rows of the subject, want %d", table, got, want)
		}
	}
}

func (w *world) exportSections(p auth.Principal, s dsr.Subject) map[string][]json.RawMessage {
	w.t.Helper()
	out, err := w.svc.Export(context.Background(), p, s)
	if err != nil {
		w.t.Fatalf("Export: %v", err)
	}
	var doc struct {
		Sections map[string][]json.RawMessage `json:"sections"`
	}
	if err := json.Unmarshal(out.JSON, &doc); err != nil {
		w.t.Fatal(err)
	}
	return doc.Sections
}

func wantCode(t *testing.T, err error, want plxerr.Code) {
	t.Helper()
	if got, _ := plxerr.CodeOf(err); got != want {
		t.Fatalf("error = %v (code %v), want code %v", err, got, want)
	}
}

// TestExport_EveryStoreOfADeviceAndAUser_SEC_162 checks that an export holds
// one section per store with exactly the subject's rows.
//
// Verifies: SEC-162.
func TestExport_EveryStoreOfADeviceAndAUser_SEC_162(t *testing.T) {
	w := newWorld(t)
	device, user := w.seedDevice(w.a), w.seedUser(w.a)
	w.seedDevice(w.a)

	sections := w.exportSections(w.a.admin, dsr.Subject{Kind: dsr.KindDevice, ID: device})
	for table := range deviceTables {
		if len(sections[table]) != 1 {
			t.Errorf("section %s has %d documents, want 1", table, len(sections[table]))
		}
	}
	sections = w.exportSections(w.a.admin, dsr.Subject{Kind: dsr.KindUser, ID: user})
	for table := range userTables {
		if len(sections[table]) != 1 {
			t.Errorf("section %s has %d documents, want 1", table, len(sections[table]))
		}
	}
}

// TestErase_RemovesEverythingAndLeavesTombstones_SEC_162 checks that an
// erasure empties every mapped store of the subject, leaves the other
// subjects alone and records a tombstone per store.
//
// Verifies: SEC-162.
func TestErase_RemovesEverythingAndLeavesTombstones_SEC_162(t *testing.T) {
	w := newWorld(t)
	device, other := w.seedDevice(w.a), w.seedDevice(w.a)
	user, otherUser := w.seedUser(w.a), w.seedUser(w.a)

	for _, c := range []struct {
		s      dsr.Subject
		other  string
		tables map[string]string
	}{
		{dsr.Subject{Kind: dsr.KindDevice, ID: device}, other, deviceTables},
		{dsr.Subject{Kind: dsr.KindUser, ID: user}, otherUser, userTables},
	} {
		got, err := w.svc.Erase(context.Background(), w.a.admin, c.s)
		if err != nil {
			t.Fatalf("Erase(%s): %v", c.s.Kind, err)
		}
		if len(got.Erased) != len(c.tables) || got.RequestID == "" {
			t.Fatalf("Erase(%s) = %+v, want %d stores", c.s.Kind, got, len(c.tables))
		}
		w.assertRows(w.a, c.tables, c.s.ID, 0)
		w.assertRows(w.a, c.tables, c.other, 1)

		req, err := w.svc.Get(context.Background(), w.a.admin, got.RequestID)
		if err != nil {
			t.Fatal(err)
		}
		if req.Action != "erase" || len(req.Tombstones) != len(c.tables) || req.SubjectHash == "" {
			t.Errorf("request = %+v, want an erasure with %d tombstones", req, len(c.tables))
		}
	}
}

// TestErase_IsIdempotent_SEC_162 checks that a second erasure changes
// nothing and reports the subject as already erased.
//
// Verifies: SEC-162.
func TestErase_IsIdempotent_SEC_162(t *testing.T) {
	w := newWorld(t)
	device := w.seedDevice(w.a)
	s := dsr.Subject{Kind: dsr.KindDevice, ID: device}
	if _, err := w.svc.Erase(context.Background(), w.a.admin, s); err != nil {
		t.Fatal(err)
	}
	again, err := w.svc.Erase(context.Background(), w.a.admin, s)
	if err != nil {
		t.Fatalf("second Erase: %v", err)
	}
	if !again.AlreadyErased || len(again.Erased) != 0 {
		t.Fatalf("second Erase = %+v, want AlreadyErased and nothing erased", again)
	}
	w.assertRows(w.a, deviceTables, device, 0)
}

// TestRequests_UnknownSubjectIsNotFound_SEC_162 checks PLX-8501 for a
// subject that never had data.
//
// Verifies: SEC-162.
func TestRequests_UnknownSubjectIsNotFound_SEC_162(t *testing.T) {
	w := newWorld(t)
	s := dsr.Subject{Kind: dsr.KindDevice, ID: w.id()}
	_, err := w.svc.Export(context.Background(), w.a.admin, s)
	wantCode(t, err, plxerr.SubjectNotFound)
	_, err = w.svc.Erase(context.Background(), w.a.admin, s)
	wantCode(t, err, plxerr.SubjectNotFound)
}

// TestRequests_RefuseAnotherOrganisation_SEC_162 checks that an
// administrator of one organisation can neither read nor erase a subject of
// another.
//
// Verifies: SEC-162.
func TestRequests_RefuseAnotherOrganisation_SEC_162(t *testing.T) {
	w := newWorld(t)
	device := w.seedDevice(w.a)
	s := dsr.Subject{Kind: dsr.KindDevice, ID: device}
	_, err := w.svc.Export(context.Background(), w.b.admin, s)
	wantCode(t, err, plxerr.SubjectNotFound)
	_, err = w.svc.Erase(context.Background(), w.b.admin, s)
	wantCode(t, err, plxerr.SubjectNotFound)
	w.assertRows(w.a, deviceTables, device, 1)
}

// TestRequests_NeedThePermission_SEC_162 checks that without dsr.manage
// nothing is exported or erased.
//
// Verifies: SEC-162.
func TestRequests_NeedThePermission_SEC_162(t *testing.T) {
	w := newWorld(t)
	device := w.seedDevice(w.a)
	s := dsr.Subject{Kind: dsr.KindDevice, ID: device}
	nobody := w.a.admin
	nobody.Permissions = nil
	noFactor := w.a.admin
	noFactor.SecondFactor = false
	if _, err := w.svc.Erase(context.Background(), noFactor, s); err == nil {
		t.Error("Erase without a second factor succeeded")
	}
	if _, err := w.svc.Export(context.Background(), nobody, s); err == nil {
		t.Error("Export without the permission succeeded")
	}
	if _, err := w.svc.Erase(context.Background(), nobody, s); err == nil {
		t.Error("Erase without the permission succeeded")
	}
	w.assertRows(w.a, deviceTables, device, 1)
}

// TestExport_RefusesPastTheLimit_SEC_162 checks that an archive over
// dsr.exportBytes is refused with PLX-1320 and not truncated.
//
// Verifies: SEC-162.
func TestExport_RefusesPastTheLimit_SEC_162(t *testing.T) {
	w := newWorld(t)
	device := w.seedDevice(w.a)
	w.budget = 64
	_, err := w.svc.Export(context.Background(), w.a.admin, dsr.Subject{Kind: dsr.KindDevice, ID: device})
	wantCode(t, err, plxerr.LimitExceeded)
}

// TestRequests_RejectMalformedSubjects_SEC_162 checks that an unknown kind
// or an identifier that is not a UUID is refused before any store is read.
//
// Verifies: SEC-162.
func TestRequests_RejectMalformedSubjects_SEC_162(t *testing.T) {
	w := newWorld(t)
	for _, s := range []dsr.Subject{{Kind: "team", ID: w.id()}, {Kind: dsr.KindDevice, ID: "not-an-id"}} {
		_, err := w.svc.Erase(context.Background(), w.a.admin, s)
		wantCode(t, err, plxerr.InvalidFormat)
	}
}
