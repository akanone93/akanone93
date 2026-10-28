// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package replay_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/replay"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

type ids struct{ g *uuid7.Generator }

func (i ids) New() (string, error) {
	u, err := i.g.New()
	return u.String(), err
}

func code(err error) plxerr.Code {
	c, _ := plxerr.CodeOf(err)
	return c
}

type world struct {
	db     *storage.DB
	gen    ids
	owner  auth.Principal
	other  auth.Principal
	app    string
	device device.Device
	org    string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	ctx := context.Background()
	db := storagetest.Open(t)
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	log := audit.NewLog(gen, nil)
	authService, err := auth.NewService(auth.Options{DB: db, Audit: log, Cache: cache.NewMemory(nil), Crypter: backend, IDs: gen, VerificationURI: "https://p.example/device"})
	if err != nil {
		t.Fatal(err)
	}
	ten, err := tenancy.NewService(tenancy.Options{DB: db, Audit: log, Auth: authService, Crypter: backend, IDs: gen, SigningKeyPrefix: "targets"})
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
	principal := func(key string) (auth.Principal, string) {
		o, err := ten.CreateOrganization(ctx, id, key, key)
		if err != nil {
			t.Fatal(err)
		}
		p, err := authService.Resolve(ctx, id, o.ID)
		if err != nil {
			t.Fatal(err)
		}
		return p, o.ID
	}
	owner, org := principal("acme")
	other, _ := principal("rival")
	app, err := ten.CreateApp(ctx, owner, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	devices, err := device.NewService(device.Options{DB: db, IDs: gen, Cache: cache.NewMemory(nil), DevelopmentProvider: true})
	if err != nil {
		t.Fatal(err)
	}
	challenge, _, err := devices.CreateChallenge(ctx, app.ID, "development")
	if err != nil {
		t.Fatal(err)
	}
	_, jwk := devicetest.NewKey(t)
	d, err := devices.RegisterAttested(ctx, device.AttestedRegistration{
		AppID: app.ID, Environment: "development", Platform: "ios", Challenge: challenge, DPoPKeyJWK: jwk,
		KeyStorage: device.KeyStorageSoftware, Evidence: device.Evidence{Development: &device.DevelopmentEvidence{BuildID: "test"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &world{db: db, gen: gen, owner: owner, other: other, app: app.ID, device: d, org: org}
}

func (w *world) source(consent bool) replay.Source {
	return replay.Source{OrganizationID: w.org, AppID: w.app, EnvironmentID: w.device.EnvironmentID, DeviceID: w.device.ID, ReplayConsent: consent}
}

func (w *world) upload(t *testing.T, started time.Time) replay.Upload {
	t.Helper()
	id, err := w.gen.New()
	if err != nil {
		t.Fatal(err)
	}
	return replay.Upload{ID: id, StartedAt: started, DurationMs: 900, Events: []replay.Event{
		{Kind: replay.KindScreen, Route: "/loans", PluginKey: "loans"},
		{AtMs: 10, Kind: replay.KindLayout, Nodes: []replay.Node{{Path: "0", Widget: "TextField", Width: 100, Height: 40}}},
		{AtMs: 20, Kind: replay.KindInput, NodePath: "0", Value: "hunter2"},
		{AtMs: 30, Kind: replay.KindState, Name: "password", ValueType: "string", Value: "hunter2"},
		{AtMs: 40, Kind: replay.KindState, Name: "count", ValueType: "int", Value: "2"},
	}}
}

// Verifies: ANL-050, SEC-161, SCH-012, LIM-001.
// A replay is stored only with the replay consent and where the security
// profile allows it, is redacted again on arrival, is bounded by the replay.*
// limits, and is listed and read by people with analytics.read on the app and
// is invisible to another organisation (row-level security).
func TestIngestListGet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t)
	now := time.Now()
	set, err := limits.Defaults().Tighten(limits.ReplayPerDevicePerDay, limits.ScopeInstallation, 3)
	if err != nil {
		t.Fatal(err)
	}
	if set, err = set.Tighten(limits.ReplayEventsPerReplay, limits.ScopeInstallation, 10); err != nil {
		t.Fatal(err)
	}
	allowed := true
	svc, err := replay.NewService(replay.Options{
		DB: w.db, Limits: set, Now: func() time.Time { return now },
		Allowed: func(context.Context, string, string) (bool, error) { return allowed, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	u := w.upload(t, now.Add(-time.Minute))
	if _, err := svc.Ingest(ctx, w.source(false), u); code(err) != plxerr.PermissionDenied {
		t.Fatalf("a replay without consent: %v", err)
	}
	allowed = false
	if _, err := svc.Ingest(ctx, w.source(true), u); code(err) != plxerr.PermissionDenied {
		t.Fatalf("a replay under a profile that forbids it: %v", err)
	}
	allowed = true
	res, err := svc.Ingest(ctx, w.source(true), u)
	if err != nil || !res.Stored || res.Dropped != 1 {
		t.Fatalf("Ingest: %+v %v", res, err)
	}
	if again, err := svc.Ingest(ctx, w.source(true), u); err != nil || again.Stored {
		t.Fatalf("a retried upload is stored once: %+v %v", again, err)
	}
	r, events, err := svc.Get(ctx, w.owner, w.app, u.ID)
	if err != nil || r.EventCount != 4 || r.Route != "/loans" || len(events) != 4 {
		t.Fatalf("Get: %+v %d %v", r, len(events), err)
	}
	for _, e := range events {
		if e.Value == "hunter2" || e.Name == "password" {
			t.Errorf("a stored event holds sensitive data: %+v", e)
		}
	}
	if !events[1].Nodes[0].Masked {
		t.Errorf("the text field is not masked: %+v", events[1].Nodes)
	}
	list, err := svc.List(ctx, w.owner, w.app, w.device.EnvironmentID, storage.Cursor{}, 10)
	if err != nil || len(list) != 1 || list[0].ID != u.ID {
		t.Fatalf("List: %+v %v", list, err)
	}
	// Another organisation can neither read nor list it, whatever it names.
	if _, _, err := svc.Get(ctx, w.other, w.app, u.ID); code(err) != plxerr.ResourceNotFound {
		t.Errorf("Get by another organisation: %v", err)
	}
	if none, err := svc.List(ctx, w.other, w.app, w.device.EnvironmentID, storage.Cursor{}, 10); err != nil || len(none) != 0 {
		t.Errorf("List by another organisation: %+v %v", none, err)
	}
	// Limits: events per replay, bytes per replay, replays per device per day.
	big := w.upload(t, now)
	big.Events = append(big.Events, make([]replay.Event, 10)...)
	if _, err := svc.Ingest(ctx, w.source(true), big); code(err) != plxerr.LimitExceeded {
		t.Errorf("too many events: %v", err)
	}
	huge := w.source(true)
	huge.Bytes = int(limits.Defaults().Get(limits.ReplayBytesPerReplay)) + 1
	if _, err := svc.Ingest(ctx, huge, w.upload(t, now)); code(err) != plxerr.IngestionOverQuota {
		t.Errorf("too many bytes: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.Ingest(ctx, w.source(true), w.upload(t, now)); err != nil {
			t.Fatalf("replay %d: %v", i+2, err)
		}
	}
	if _, err := svc.Ingest(ctx, w.source(true), w.upload(t, now)); code(err) != plxerr.IngestionOverQuota {
		t.Errorf("the fourth replay of the day: %v", err)
	}
	if _, err := svc.Ingest(ctx, w.source(true), replay.Upload{ID: "x", StartedAt: now}); code(err) != plxerr.InvalidFormat {
		t.Errorf("a bad identifier: %v", err)
	}
	long := w.upload(t, now.Add(-30*24*time.Hour))
	if _, err := svc.Ingest(ctx, w.source(true), long); code(err) != plxerr.OutOfRange && code(err) != plxerr.IngestionOverQuota {
		t.Errorf("a replay older than the retention: %v", err)
	}
}

// Verifies: ANL-050.
// The retention sweep deletes replays whose retention has passed and keeps the
// others; an expired replay is no longer listed or read.
func TestRetention(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t)
	clock := time.Now()
	svc, err := replay.NewService(replay.Options{DB: w.db, Now: func() time.Time { return clock }})
	if err != nil {
		t.Fatal(err)
	}
	u := w.upload(t, clock)
	if _, err := svc.Ingest(ctx, w.source(true), u); err != nil {
		t.Fatal(err)
	}
	days := time.Duration(limits.Defaults().Get(limits.ReplayRetentionDays)) * 24 * time.Hour
	clock = clock.Add(days - time.Hour)
	if n, err := svc.Purge(ctx, w.org); err != nil || n != 0 {
		t.Fatalf("Purge before the retention: %d %v", n, err)
	}
	clock = clock.Add(2 * time.Hour)
	if _, _, err := svc.Get(ctx, w.owner, w.app, u.ID); code(err) != plxerr.ResourceNotFound {
		t.Errorf("Get of an expired replay: %v", err)
	}
	if n, err := svc.Purge(ctx, w.org); err != nil || n != 1 {
		t.Fatalf("Purge after the retention: %d %v", n, err)
	}
}
