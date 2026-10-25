// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package preferences_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/preferences"
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

type fixture struct {
	svc     *preferences.Service
	tenancy *tenancy.Service
	auth    *auth.Service
	id      auth.Identity
	org     string
	owner   auth.Principal
	app     string
	other   auth.Principal
	second  auth.Principal
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	db := storagetest.Open(t)
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	log := audit.NewLog(gen, nil)
	a, err := auth.NewService(auth.Options{
		DB: db, Audit: log, Cache: cache.NewMemory(nil), Crypter: backend, IDs: gen, VerificationURI: "https://p.example/device",
	})
	if err != nil {
		t.Fatal(err)
	}
	ten, err := tenancy.NewService(tenancy.Options{
		DB: db, Audit: log, Auth: a, Crypter: backend, IDs: gen, SigningKeyPrefix: "targets",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := preferences.NewService(preferences.Options{DB: db, Tenancy: ten})
	if err != nil {
		t.Fatal(err)
	}
	admin, invitation, err := a.Bootstrap(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AcceptInvitation(ctx, invitation, "Admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	id := auth.Identity{Kind: auth.KindUser, ID: admin.ID, UserID: admin.ID, Display: "Admin", SecondFactor: true, InstallationAdmin: true}.WithStepUp(0)
	f := &fixture{svc: svc, tenancy: ten, auth: a, id: id}
	f.org, f.owner = f.organisation(t, "acme", "Acme")
	app, err := ten.CreateApp(ctx, f.owner, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	f.app = app.ID
	_, f.other = f.organisation(t, "globex", "Globex")
	var second auth.User
	err = db.InTx(ctx, storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		second, _, err = a.Invite(ctx, tx, "second@example.com")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	f.second = f.owner
	f.second.Identity = auth.Identity{Kind: auth.KindUser, ID: second.ID, UserID: second.ID, Display: "Second"}
	return f
}

func (f *fixture) organisation(t *testing.T, key, name string) (string, auth.Principal) {
	t.Helper()
	ctx := context.Background()
	o, err := f.tenancy.CreateOrganization(ctx, f.id, key, name)
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.auth.Resolve(ctx, f.id, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	return o.ID, p
}

func TestPreferencesRoundTrip_STU_014(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	got, err := f.svc.Get(ctx, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != preferences.ThemeSystem || got.Density != preferences.DensityComfortable {
		t.Fatalf("defaults = %+v", got)
	}
	in := preferences.Preferences{Theme: preferences.ThemeDark, Language: "de-ch", Shortcuts: map[string]string{"save": "Mod+S"}, Pinned: []string{"/a"}}
	if _, err := f.svc.Update(ctx, f.owner, in, []string{"theme", "language", "shortcuts", "pinned"}); err != nil {
		t.Fatal(err)
	}
	got, err = f.svc.Get(ctx, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "dark" || got.Language != "de-CH" || got.Shortcuts["save"] != "Mod+S" || len(got.Pinned) != 1 {
		t.Fatalf("round trip = %+v", got)
	}
}

func TestPreferencesAcceptContrastTheme_STU_014(t *testing.T) {
	f := newFixture(t)
	got, err := f.svc.Update(context.Background(), f.owner, preferences.Preferences{Theme: "contrast"}, []string{"theme"})
	if err != nil || got.Theme != preferences.ThemeContrast {
		t.Fatalf("contrast theme: %+v, %v", got, err)
	}
}

func TestPreferencesMaskLeavesOthers_STU_014(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.Update(ctx, f.owner, preferences.Preferences{Theme: "dark", Density: "compact"}, []string{"theme", "density"}); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Update(ctx, f.owner, preferences.Preferences{Theme: "light", Density: "comfortable"}, []string{"theme"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "light" || got.Density != "compact" {
		t.Fatalf("mask leaked: %+v", got)
	}
}

func TestPreferencesRefused_STU_014(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	over := make([]string, 0)
	for range 100000 {
		over = append(over, f.app)
	}
	cases := map[string]struct {
		in   preferences.Preferences
		mask []string
	}{
		"empty mask":    {preferences.Preferences{}, nil},
		"unknown path":  {preferences.Preferences{}, []string{"nope"}},
		"invalid theme": {preferences.Preferences{Theme: "neon"}, []string{"theme"}},
		"bad language":  {preferences.Preferences{Language: "not a tag!"}, []string{"language"}},
		"favourites":    {preferences.Preferences{FavouriteAppIDs: over}, []string{"favourite_app_ids"}},
		"pinned":        {preferences.Preferences{Pinned: over}, []string{"pinned"}},
		"text scale":    {preferences.Preferences{Preview: map[string]preferences.PreviewSwitches{f.app: {TextScale: -1}}}, []string{"preview"}},
		"preview key":   {preferences.Preferences{Preview: map[string]preferences.PreviewSwitches{"x": {}}}, []string{"preview"}},
	}
	for name, c := range cases {
		if _, err := f.svc.Update(ctx, f.owner, c.in, c.mask); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestPreferencesIsolation_STU_014(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.Update(ctx, f.owner, preferences.Preferences{Theme: "dark"}, []string{"theme"}); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]auth.Principal{"other organisation": f.other, "other user": f.second} {
		got, err := f.svc.Get(ctx, p)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Theme != preferences.ThemeSystem {
			t.Errorf("%s sees theme %q", name, got.Theme)
		}
	}
}

func TestPreviewSwitchesRoundTrip_STU_058(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sw := preferences.PreviewSwitches{DeviceFrame: "pixel", Theme: "dark", Locale: "fr", TextScale: 1.3, Platform: "cupertino", MockStates: map[string]string{"cart": "empty"}}
	in := preferences.Preferences{Preview: map[string]preferences.PreviewSwitches{f.app: sw}}
	if _, err := f.svc.Update(ctx, f.owner, in, []string{"preview"}); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Get(ctx, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	g := got.Preview[f.app]
	if g.DeviceFrame != "pixel" || g.TextScale != 1.3 || g.Platform != "cupertino" || g.MockStates["cart"] != "empty" || g.Locale != "fr" {
		t.Fatalf("preview = %+v", g)
	}
}

func TestOnboarding_STU_015(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, empty := f.organisation(t, "initech", "Initech")
	done := func(p auth.Principal) bool {
		steps, err := f.svc.Onboarding(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range steps {
			if s.Key == preferences.StepCreateApp {
				return s.Done
			}
		}
		t.Fatal("no create_app step")
		return false
	}
	if done(empty) {
		t.Fatal("create_app done in an organisation without apps")
	}
	if _, err := f.tenancy.CreateApp(ctx, empty, "first", "First"); err != nil {
		t.Fatal(err)
	}
	if !done(empty) {
		t.Fatal("create_app not done after creating an app")
	}
}
