// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth_test

import (
	"context"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// releaseOnDevelopment is the mapping rule that gives a group the release
// manager role on the development environment and the web team.
func releaseOnDevelopment() auth.MappingRule {
	return auth.MappingRule{
		Group: "eng-web",
		Grants: []auth.GrantRule{{
			Role: string(auth.RoleReleaseManager), Scope: auth.ScopeEnvironment, App: "shop", Environment: "development",
		}},
		Teams: []string{"web"},
	}
}

// assignments counts what the provider's mapping gave a person: grants and
// team memberships.
func (f *federated) assignments(t *testing.T, userID string) int {
	t.Helper()
	var n int
	scanIn(t, f.fixture, storage.Tenant{OrganizationID: f.org}, `SELECT count(*) FROM federated_assignments WHERE provider_id = $1 AND user_id = $2`,
		[]any{&n}, f.provider.ID, userID)
	return n
}

// inTeam reports whether a person is a member of the web team.
func (f *federated) inTeam(t *testing.T, userID string) bool {
	t.Helper()
	var n int
	scanIn(t, f.fixture, storage.Tenant{OrganizationID: f.org}, `SELECT count(*) FROM memberships WHERE organization_id = $1 AND user_id = $2 AND team_id = $3`,
		[]any{&n}, f.org, userID, f.team)
	return n > 0
}

// Verifies: GOV-004, GOV-003.
// A sign-in gives a person exactly the scoped grants and team memberships
// their groups map to. Unmapped groups grant nothing, and a changed mapping
// changes them at the next sign-in.
func TestGroupMapping_GOV_004(t *testing.T) {
	t.Parallel()
	f := newFederated(t)
	f.createProvider(t, false)
	f.setMapping(t, auth.GroupMapping{Rules: []auth.MappingRule{releaseOnDevelopment(), {
		Group:  "finance",
		Grants: []auth.GrantRule{{Role: string(auth.RoleDeveloper), Scope: auth.ScopeApp, App: "shop"}},
	}}})
	ctx := context.Background()
	if got, err := f.svc.GetMapping(ctx, f.ownerScope(t), f.provider.ID); err != nil || len(got.Rules) != 2 {
		t.Fatalf("GetMapping = %+v, %v", got, err)
	}

	f.mustSignIn(t, "sub-bob", "bob@example.com", map[string]any{"groups": []string{"eng-web", "unmapped"}})
	if n := f.assignments(t, f.bobID); n != 2 {
		t.Errorf("a sign-in with one mapped group gave %d assignments; want a grant and a team membership", n)
	}
	bob := f.principalOf(t, "bob@example.com", f.org)
	if err := bob.AuthorizeEnvironment(auth.ReleasePublish, f.app, "development"); err != nil {
		t.Errorf("publishing to development after the mapping: %v", err)
	}
	if err := bob.AuthorizeEnvironment(auth.ReleasePublish, f.app, "production"); code(err) != plxerr.PermissionDenied {
		t.Errorf("publishing to production: %v", err)
	}
	if !f.inTeam(t, f.bobID) {
		t.Error("the mapped team membership is missing")
	}

	f.setMapping(t, auth.GroupMapping{Rules: []auth.MappingRule{{
		Group:  "eng-web",
		Grants: []auth.GrantRule{{Role: string(auth.RoleReleaseManager), Scope: auth.ScopeEnvironment, App: "shop", Environment: "production"}},
	}}})
	f.mustSignIn(t, "sub-bob", "bob@example.com", map[string]any{"groups": []string{"eng-web"}})
	if n := f.assignments(t, f.bobID); n != 1 {
		t.Errorf("after the mapping changed, %d assignments; want one grant", n)
	}
	if f.inTeam(t, f.bobID) {
		t.Error("a team the mapping no longer names is still joined")
	}
	bob = f.principalOf(t, "bob@example.com", f.org)
	if err := bob.AuthorizeEnvironment(auth.ReleasePublish, f.app, "development"); code(err) != plxerr.PermissionDenied {
		t.Errorf("the grant on development outlived the mapping: %v", err)
	}
	if err := bob.AuthorizeEnvironment(auth.ReleasePublish, f.app, "production"); err != nil {
		t.Errorf("publishing to production after the mapping changed: %v", err)
	}

	f.mustSignIn(t, "sub-bob", "bob@example.com", map[string]any{"groups": []string{"unmapped"}})
	if n := f.assignments(t, f.bobID); n != 0 {
		t.Errorf("a sign-in with no mapped group kept %d assignments", n)
	}
	bob = f.principalOf(t, "bob@example.com", f.org)
	if err := bob.AuthorizeEnvironment(auth.ReleasePublish, f.app, "production"); code(err) != plxerr.PermissionDenied {
		t.Errorf("a person with no mapped group still publishes: %v", err)
	}
}

// Verifies: GOV-004.
// A mapping names roles that exist, teams that exist and scopes that exist.
// Anything else is refused, so that a mapping cannot grant what cannot be
// given.
func TestMappingRefusesWhatCannotBeGiven_GOV_004(t *testing.T) {
	t.Parallel()
	f := newFederated(t)
	f.createProvider(t, false)
	ctx := context.Background()
	sc := f.ownerScope(t)
	for name, rule := range map[string]auth.MappingRule{
		"an unknown team": {Group: "x", Teams: []string{"no-such-team"}},
		"an unknown app": {Group: "x", Grants: []auth.GrantRule{{
			Role: string(auth.RoleDeveloper), Scope: auth.ScopeApp, App: "no-such-app",
		}}},
		"an unknown environment": {Group: "x", Grants: []auth.GrantRule{{
			Role: string(auth.RoleReleaseManager), Scope: auth.ScopeEnvironment, App: "shop", Environment: "staging",
		}}},
		"an unknown role": {Group: "x", Grants: []auth.GrantRule{{
			Role: "no-such-role", Scope: auth.ScopeOrganization,
		}}},
	} {
		if _, err := f.svc.SetMapping(ctx, sc, f.provider.ID, auth.GroupMapping{Rules: []auth.MappingRule{rule}}); err == nil {
			t.Errorf("a mapping with %s was accepted", name)
		}
	}
	if got, err := f.svc.GetMapping(ctx, sc, f.provider.ID); err != nil || len(got.Rules) != 0 {
		t.Errorf("refused mappings were kept: %+v, %v", got, err)
	}
}

// Verifies: GOV-004, SEC-100.
// Just-in-time provisioning is off by default: a person the provider vouches
// for but who has no account is refused. Switched on, the person is created
// with only what the mapping gives. An existing account outside the
// organisation is never linked, whatever the setting.
func TestJustInTimeProvisioning_GOV_004(t *testing.T) {
	t.Parallel()
	f := newFederated(t)
	f.createProvider(t, false)
	f.setMapping(t, auth.GroupMapping{Rules: []auth.MappingRule{releaseOnDevelopment()}})
	ctx := context.Background()
	f.person(t, "dan@example.com")

	if _, _, err := f.federatedSignIn(t, "sub-newbie", "newbie@example.com", true, map[string]any{"groups": []string{"eng-web"}}); code(err) != plxerr.AuthenticationRequired {
		t.Errorf("an unknown person with provisioning off: %v", err)
	}
	if n := f.countUsers(t, "newbie@example.com"); n != 0 {
		t.Errorf("an account was created with provisioning off: %d", n)
	}

	f.setJIT(t, true)
	if _, _, err := f.federatedSignIn(t, "sub-dan", "dan@example.com", true, map[string]any{"groups": []string{"eng-web"}}); code(err) != plxerr.AuthenticationRequired {
		t.Errorf("an account outside the organisation was linked: %v", err)
	}
	if _, _, err := f.federatedSignIn(t, "sub-newbie", "newbie@example.com", false, nil); code(err) != plxerr.AuthenticationRequired {
		t.Errorf("an unverified address was provisioned: %v", err)
	}

	session := f.mustSignIn(t, "sub-newbie", "newbie@example.com", map[string]any{"groups": []string{"eng-web"}})
	if session.Secret == "" {
		t.Fatal("a provisioned person has no session")
	}
	id, err := f.svc.AuthenticateSession(ctx, session.Secret)
	if err != nil {
		t.Fatalf("AuthenticateSession: %v", err)
	}
	if n := f.assignments(t, id.UserID); n != 2 {
		t.Errorf("a provisioned person has %d assignments; want exactly the mapping's grant and team", n)
	}
	if !hasAudit(auditEntries(t, f.fixture, ""), audit.UserProvisioned) {
		t.Error("no user.provisioned entry in the installation's chain")
	}

	second := f.mustSignIn(t, "sub-second", "second@example.com", nil)
	secondID, err := f.svc.AuthenticateSession(ctx, second.Secret)
	if err != nil {
		t.Fatalf("AuthenticateSession: %v", err)
	}
	if n := f.countUsers(t, "second@example.com"); n != 1 {
		t.Fatalf("a provisioned person without groups: %d accounts", n)
	}
	if n := f.assignments(t, secondID.UserID); n != 0 {
		t.Errorf("a provisioned person without groups has %d assignments", n)
	}
}

// countUsers counts the accounts with an address.
func (f *federated) countUsers(t *testing.T, email string) int {
	t.Helper()
	var n int
	if err := f.db.Pool().QueryRow(context.Background(), `SELECT count(*) FROM users WHERE email = $1`, email).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
