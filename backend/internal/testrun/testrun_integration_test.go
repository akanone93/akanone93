// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package testrun_test

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
	"github.com/nightCode42/plux3/backend/internal/testrun"
)

type ids struct{ g *uuid7.Generator }

func (i ids) New() (string, error) {
	u, err := i.g.New()
	return u.String(), err
}

type fixture struct {
	svc            *testrun.Service
	ownerA, ownerB auth.Principal
	viewer         auth.Principal
	appA, appB     string
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
	authService, err := auth.NewService(auth.Options{DB: db, Audit: log, Cache: cache.NewMemory(nil), Crypter: backend, IDs: gen, VerificationURI: "https://p.example/device"})
	if err != nil {
		t.Fatal(err)
	}
	ten, err := tenancy.NewService(tenancy.Options{DB: db, Audit: log, Auth: authService, Crypter: backend, IDs: gen, SigningKeyPrefix: "targets"})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{svc: testrun.New(testrun.Options{DB: db, IDs: gen})}
	admin, invitation, err := authService.Bootstrap(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authService.AcceptInvitation(ctx, invitation, "Admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	adminID := auth.Identity{Kind: auth.KindUser, ID: admin.ID, UserID: admin.ID, Display: "Admin", SecondFactor: true, InstallationAdmin: true}.WithStepUp(0)
	resolve := func(id auth.Identity, org string) auth.Principal {
		p, err := authService.Resolve(ctx, id, org)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	orgA, err := ten.CreateOrganization(ctx, adminID, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := ten.CreateOrganization(ctx, adminID, "beta", "Beta")
	if err != nil {
		t.Fatal(err)
	}
	f.ownerA, f.ownerB = resolve(adminID, orgA.ID), resolve(adminID, orgB.ID)
	m, _, err := ten.AddMember(ctx, f.ownerA, "", "viewer@example.com", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	f.viewer = resolve(auth.Identity{Kind: auth.KindUser, ID: m.UserID, UserID: m.UserID, Display: "viewer", SecondFactor: true}.WithStepUp(0), orgA.ID)
	appA, err := ten.CreateApp(ctx, f.ownerA, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	appB, err := ten.CreateApp(ctx, f.ownerB, "other", "Other")
	if err != nil {
		t.Fatal(err)
	}
	f.appA, f.appB = appA.ID, appB.ID
	return f
}

func codeIs(err error, code plxerr.Code) bool {
	return err != nil && errors.Is(err, plxerr.New(code, ""))
}

func report(app, key string) testrun.Report {
	return testrun.Report{AppID: app, Key: key, Source: "ci", Label: "main", Scenarios: []testrun.Scenario{
		{ID: "checkout", Name: "Checkout", Status: testrun.StatusFailed, DurationMs: 40, Steps: []testrun.Step{
			{Index: 0, Description: "tap checkout.pay", Status: testrun.StatusPassed},
			{Index: 1, Description: "expect text Paid", Status: testrun.StatusFailed, Message: "found Failed"},
		}},
		{ID: "empty-cart", Name: "Empty cart", Status: testrun.StatusPassed, DurationMs: 10},
	}}
}

// Verifies: TST-002.
// A run is reported with its scenario and step results, is idempotent per
// report key, is listed newest first and is read back whole.
func TestReport_ListAndGet_TST_002(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	first, err := f.svc.Report(ctx, f.ownerA, report(f.appA, "run-1"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Passed != 1 || first.Failed != 1 || first.DurationMs != 50 || len(first.Scenarios) != 2 {
		t.Fatalf("run = %+v", first)
	}
	again, err := f.svc.Report(ctx, f.ownerA, report(f.appA, "run-1"))
	if err != nil || again.ID != first.ID {
		t.Fatalf("a repeated report must return the first run: %v %v", again.ID, err)
	}
	second, err := f.svc.Report(ctx, f.ownerA, report(f.appA, "run-2"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := f.svc.List(ctx, f.viewer, f.appA, storage.Cursor{}, 10)
	if err != nil || len(list) != 2 || list[0].ID != second.ID || len(list[0].Scenarios) != 0 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	got, err := f.svc.Get(ctx, f.viewer, f.appA, first.ID)
	if err != nil || got.Scenarios[0].Steps[1].Message != "found Failed" {
		t.Fatalf("get = %+v, %v", got, err)
	}
}

// Verifies: TST-002, SEC-080.
// Reporting needs plugin.edit, and a run of another organisation is neither
// listed, read nor reported to.
func TestReport_Tenancy_TST_002(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.Report(ctx, f.viewer, report(f.appA, "k")); err == nil {
		t.Fatal("a viewer must not report a run")
	}
	run, err := f.svc.Report(ctx, f.ownerA, report(f.appA, "k"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Get(ctx, f.ownerB, f.appA, run.ID); err == nil {
		t.Fatal("another organisation read a run")
	}
	if runs, err := f.svc.List(ctx, f.ownerB, f.appA, storage.Cursor{}, 10); err == nil && len(runs) != 0 {
		t.Fatal("another organisation listed runs")
	}
	if _, err := f.svc.Report(ctx, f.ownerB, report(f.appA, "x")); err == nil {
		t.Fatal("another organisation reported into an app")
	}
	if _, err := f.svc.Get(ctx, f.ownerB, f.appB, run.ID); !codeIs(err, plxerr.ResourceNotFound) {
		t.Fatalf("a run is not found through another app: %v", err)
	}
}

// Verifies: TST-002, LIM-001.
// A malformed report and one over test.stepsPerRun are refused.
func TestReport_Validation_TST_002(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	bad := report(f.appA, "")
	if _, err := f.svc.Report(ctx, f.ownerA, bad); !codeIs(err, plxerr.InvalidFormat) {
		t.Fatalf("empty key: %v", err)
	}
	bad = report(f.appA, "k")
	bad.Scenarios[0].Status = "weird"
	if _, err := f.svc.Report(ctx, f.ownerA, bad); !codeIs(err, plxerr.InvalidFormat) {
		t.Fatalf("status: %v", err)
	}
	big := report(f.appA, "big")
	big.Scenarios[0].Steps = make([]testrun.Step, 5001)
	for i := range big.Scenarios[0].Steps {
		big.Scenarios[0].Steps[i] = testrun.Step{Status: testrun.StatusPassed, Description: strings.Repeat("a", 1)}
	}
	if _, err := f.svc.Report(ctx, f.ownerA, big); !codeIs(err, plxerr.LimitExceeded) {
		t.Fatalf("limit: %v", err)
	}
}
