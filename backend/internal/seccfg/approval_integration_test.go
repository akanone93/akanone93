// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package seccfg_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/seccfg"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// reviewer adds a member with the reviewer role, who may decide approvals.
func (f *fixture) reviewer(t *testing.T, email string) auth.Principal {
	t.Helper()
	m, _, err := f.tenancy.AddMember(context.Background(), f.owner, "", email, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.auth.Resolve(context.Background(), auth.Identity{Kind: auth.KindUser, ID: m.UserID, UserID: m.UserID, Display: email, SecondFactor: true}.WithStepUp(0), f.org)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// guarded returns a configuration service whose changes ask the approval
// engine, which reads each environment's profile from the service itself.
func (f *fixture) guarded(t *testing.T) (*seccfg.Service, *approval.Service) {
	t.Helper()
	engine, err := approval.New(approval.Options{DB: f.db, Audit: f.log, IDs: f.gen})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := seccfg.New(seccfg.Options{DB: f.db, Audit: f.log, Limits: limits.Defaults(), Approvals: engine})
	if err != nil {
		t.Fatal(err)
	}
	engine.Use(func(ctx context.Context, app, env string) (settings.Profile, error) {
		v, _, err := svc.Effective(ctx, app, env)
		return v.Profile(), err
	}, nil)
	return svc, engine
}

// refusedFor returns the identifier of the request a refusal names.
func refusedFor(t *testing.T, err error) string {
	t.Helper()
	var e *plxerr.Error
	if !errors.As(err, &e) || e.Code != plxerr.ApprovalRequired || e.Details["request"] == "" {
		t.Fatalf("want an approval to be required, got %v", err)
	}
	return e.Details["request"]
}

// Verifies: SEC-182, FN-055, GOV-020, GOV-021.
// A change of the security configuration needs four-eyes approval once the
// environment is under the maximum profile, and turning device-placed
// functions on needs a second approval of its own; under the standard
// profile nothing is asked, and an unchanged configuration asks nothing.
func TestSecurityConfigurationChangeNeedsApproval_SEC_182(t *testing.T) {
	f := newFixture(t, limits.Defaults())
	ctx := context.Background()
	svc, engine := f.guarded(t)
	first, second := f.reviewer(t, "r1@example.com"), f.reviewer(t, "r2@example.com")
	set := func(env, profile, overrides string, expected int64) (int64, error) {
		return svc.Set(ctx, f.owner, seccfg.Change{AppID: f.app, EnvironmentID: f.envs[env], Profile: profile, Overrides: []byte(overrides), ExpectedVersion: expected})
	}
	approve := func(err error) {
		t.Helper()
		id := refusedFor(t, err)
		for _, p := range []auth.Principal{first, second} {
			if _, err := engine.Decide(ctx, p, f.app, id, approval.DecisionApprove, ""); err != nil {
				t.Fatal(err)
			}
		}
	}

	if _, err := set("development", "standard", `{"inactivityLockTimeout": 200}`, 0); err != nil {
		t.Fatalf("an environment under the standard profile asks nothing: %v", err)
	}
	if _, err := set("production", "maximum", "", 0); err != nil {
		t.Fatalf("under the standard profile nothing is asked: %v", err)
	}
	if v, err := set("production", "maximum", "", 1); err != nil || v != 1 {
		t.Fatalf("setting what is already there asks nothing and changes nothing: %d %v", v, err)
	}
	_, err := set("production", "maximum", `{"inactivityLockTimeout": 120}`, 1)
	approve(err)
	if v, err := set("production", "maximum", `{"inactivityLockTimeout": 120}`, 1); err != nil || v != 2 {
		t.Fatalf("the approved change: %d %v", v, err)
	}
	if _, err := set("production", "maximum", `{"inactivityLockTimeout": 120}`, 2); err != nil {
		t.Errorf("setting what is already there: %v", err)
	}
	if _, err := set("production", "maximum", `{"inactivityLockTimeout": 90}`, 2); err == nil {
		t.Error("a second change on a spent approval was allowed")
	}

	// Leaving the maximum profile turns device-placed functions on (FN-055):
	// a second request of its own, opened beside the change of configuration.
	_, err = set("production", "strict", "", 2)
	refusedFor(t, err)
	pending, err := engine.ListApprovals(ctx, f.owner, f.app, approval.StatusOpen, storage.Cursor{}, 20)
	if err != nil {
		t.Fatal(err)
	}
	enabling := 0
	for _, r := range pending {
		if r.Action == approval.ActionEnableDeviceFunctions {
			enabling++
		}
		for _, p := range []auth.Principal{first, second} {
			if _, err := engine.Decide(ctx, p, f.app, r.ID, approval.DecisionApprove, ""); err != nil {
				t.Fatal(err)
			}
		}
	}
	if enabling != 1 {
		t.Fatalf("%d requests to enable device functions, want 1", enabling)
	}
	if v, err := set("production", "strict", "", 2); err != nil || v != 3 {
		t.Fatalf("enabling device functions after both approvals: %d %v", v, err)
	}
}
