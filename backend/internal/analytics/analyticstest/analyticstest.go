// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package analyticstest builds a database with an organisation, an app and
// registered devices, together with the ingestion and analytics services on
// a clock a test moves, for the tests of telemetry and analytics storage
// (ANL-010, ANL-011).
package analyticstest

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
	"github.com/nightCode42/plux3/backend/internal/telemetry"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// Clock is a clock that moves only when a test moves it.
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

// Now returns the time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Set moves the clock to t.
func (c *Clock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

// Advance moves the clock forward by d.
func (c *Clock) Advance(d time.Duration) { c.Set(c.Now().Add(d)) }

// IDs generates identifiers.
type IDs struct{ g *uuid7.Generator }

// New returns a fresh identifier.
func (i IDs) New() (string, error) {
	u, err := i.g.New()
	return u.String(), err
}

// Options changes how a Fixture is built.
type Options struct {
	// Start is the clock's first reading; the default is 2026-10-19 10:00 UTC.
	Start time.Time
	// Limits replaces the default limits.
	Limits limits.Set
	// Window is the maintenance window; the default is 02:00 to 04:00 UTC.
	Window *analytics.Window
	// Export configures the OTLP export.
	Export analytics.ExportOptions
	// Defaults are the installation's retention defaults; the zero value is
	// 396 days for raw events and 3653 for the production audit log.
	Defaults analytics.Defaults
	// Exporter runs the Parquet and webhook exports; nil leaves them off.
	Exporter analytics.Exporter
	// Objects is where the Parquet exporter puts its files.
	Objects objects.Store
}

// Fixture is a database with one organisation, one app and one device, and
// the services built on them.
type Fixture struct {
	DB    *storage.DB
	Clock *Clock
	IDs   IDs
	Audit *audit.Log
	// Owner holds every permission, with a second factor.
	Owner auth.Principal
	Org   string
	App   string
	// Env is the id of the development environment, Device the first device.
	Env    string
	Device string
	// Limits are the limits the services run with.
	Limits    limits.Set
	Devices   *device.Service
	Store     *analytics.Postgres
	Analytics *analytics.Service
	// Telemetry writes straight to the database; Ingestion builds others.
	Telemetry *telemetry.Service
}

// New builds a fixture.
func New(t *testing.T, o Options) *Fixture {
	t.Helper()
	ctx := context.Background()
	if o.Start.IsZero() {
		o.Start = time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	}
	if o.Limits.IsZero() {
		o.Limits = limits.Defaults()
	}
	if o.Defaults == (analytics.Defaults{}) {
		o.Defaults = analytics.Defaults{RawDays: 396, AuditDays: 3653}
	}
	f := &Fixture{Clock: &Clock{t: o.Start}, DB: storagetest.Open(t), Limits: o.Limits}
	f.IDs = IDs{g: uuid7.NewGenerator(f.Clock.Now, rand.Reader)}
	f.Audit = audit.NewLog(f.IDs, nil)
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(auth.Options{DB: f.DB, Audit: f.Audit, Cache: cache.NewMemory(nil), Crypter: backend, IDs: f.IDs, VerificationURI: "https://p.example/device"})
	if err != nil {
		t.Fatal(err)
	}
	ten, err := tenancy.NewService(tenancy.Options{DB: f.DB, Audit: f.Audit, Auth: authService, Crypter: backend, IDs: f.IDs, SigningKeyPrefix: "targets"})
	if err != nil {
		t.Fatal(err)
	}
	admin, invitation, err := authService.Bootstrap(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authService.AcceptInvitation(ctx, invitation, "Admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	id := auth.Identity{Kind: auth.KindUser, ID: admin.ID, UserID: admin.ID, Display: "Admin", SecondFactor: true, InstallationAdmin: true}.WithStepUp(0)
	org, err := ten.CreateOrganization(ctx, id, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	f.Org = org.ID
	if f.Owner, err = authService.Resolve(ctx, id, org.ID); err != nil {
		t.Fatal(err)
	}
	app, err := ten.CreateApp(ctx, f.Owner, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	f.App = app.ID
	f.Devices, err = device.NewService(device.Options{DB: f.DB, IDs: f.IDs, Cache: cache.NewMemory(nil), DevelopmentProvider: true})
	if err != nil {
		t.Fatal(err)
	}
	f.Device, f.Env = f.NewDevice(t)
	f.Store = analytics.NewPostgres(f.DB, f.Clock.Now)
	f.Telemetry = f.Ingestion(t, telemetry.Options{})
	window := o.Window
	if window == nil {
		w, err := analytics.ParseWindow("02:00", "04:00", "UTC")
		if err != nil {
			t.Fatal(err)
		}
		window = &w
	}
	f.Analytics, err = analytics.New(analytics.Options{
		DB: f.DB, Store: f.Store, Audit: f.Audit, IDs: f.IDs, Limits: o.Limits, Now: f.Clock.Now, Defaults: o.Defaults, Window: *window,
		Organizations: func(ctx context.Context, fn func(org string) error) error { return fn(f.Org) }, Export: o.Export,
		Exporter: o.Exporter, Objects: o.Objects, Crypter: backend,
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// NewDevice registers another device of the app's development environment
// and returns its id and its environment's id.
func (f *Fixture) NewDevice(t *testing.T) (deviceID, environmentID string) {
	t.Helper()
	ctx := context.Background()
	challenge, _, err := f.Devices.CreateChallenge(ctx, f.App, "development")
	if err != nil {
		t.Fatal(err)
	}
	_, jwk := devicetest.NewKey(t)
	d, err := f.Devices.RegisterAttested(ctx, device.AttestedRegistration{
		AppID: f.App, Environment: "development", Platform: "ios", Challenge: challenge, DPoPKeyJWK: jwk,
		KeyStorage: device.KeyStorageSoftware, Evidence: device.Evidence{Development: &device.DevelopmentEvidence{BuildID: "test"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return d.ID, d.EnvironmentID
}

// Ingestion builds an ingestion service on the fixture's database, clock,
// limits and store; o may set the queue, the counter and the limits.
func (f *Fixture) Ingestion(t *testing.T, o telemetry.Options) *telemetry.Service {
	t.Helper()
	o.DB, o.IDs, o.Now = f.DB, f.IDs, f.Clock.Now
	if o.Limits.IsZero() {
		o.Limits = f.Limits
	}
	if o.Store == nil && f.Store != nil {
		o.Store = f.Store
	}
	s, err := telemetry.NewService(o)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Source is the sender of a batch from a device of the fixture.
func (f *Fixture) Source(deviceID string) telemetry.Source {
	return telemetry.Source{OrganizationID: f.Org, AppID: f.App, EnvironmentID: f.Env, DeviceID: deviceID}
}

// Viewer is a principal that may read the app and its analytics, and nothing
// else.
func (f *Fixture) Viewer() auth.Principal {
	return auth.Principal{
		Identity:       auth.Identity{Kind: auth.KindUser, ID: f.Owner.UserID, UserID: f.Owner.UserID, Display: "Viewer"},
		OrganizationID: f.Org,
		Permissions:    []auth.Permission{auth.AppRead, auth.AnalyticsRead},
	}
}

// Count returns how many raw events of a name are stored; an empty name
// counts all.
func (f *Fixture) Count(t *testing.T, name string) int {
	t.Helper()
	var n int
	err := f.DB.InTx(context.Background(), storage.Tenant{OrganizationID: f.Org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM analytics_events WHERE $1 = '' OR name = $1", name).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Partitions lists the names of the raw-event partitions, in order.
func (f *Fixture) Partitions(t *testing.T) []string {
	t.Helper()
	var out []string
	err := f.DB.InTx(context.Background(), storage.Tenant{}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT c.relname FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
			WHERE i.inhparent = 'analytics_events'::regclass ORDER BY c.relname`)
		if err != nil {
			return err //nolint:wrapcheck // a test helper
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err //nolint:wrapcheck // a test helper
			}
			out = append(out, name)
		}
		return rows.Err() //nolint:wrapcheck // a test helper
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
