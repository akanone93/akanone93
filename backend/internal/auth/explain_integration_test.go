// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth_test

import (
	"context"
	"slices"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// unknownTeam is a well-formed identifier no team has.
const unknownTeam = "018f0000-0000-7000-8000-000000000000"

// effective returns a principal's effective permissions in a scope.
func effective(t *testing.T, p auth.Principal, scope auth.AccessScope) auth.EffectivePermissions {
	t.Helper()
	set, err := p.Effective(scope)
	if err != nil {
		t.Fatalf("Effective(%+v): %v", scope, err)
	}
	return set
}

// Verifies: GOV-007, GOV-003.
// The effective set is one sorted, versioned set per (app, environment): a
// grant on an app reaches that app and its environments only, a grant on an
// environment that environment only, and a restriction on an app takes its
// permissions away there and nowhere else.
func TestEffectivePermissionsPerScope_GOV_007(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	blog := g.secondApp(t)
	if _, err := g.svc.CreateGrant(ctx, g.owner, auth.GrantSpec{UserID: g.bobID, Role: string(auth.RoleDeveloper), Scope: auth.ScopeApp, AppID: g.app}); err != nil {
		t.Fatalf("app grant: %v", err)
	}
	if _, err := g.svc.CreateGrant(ctx, g.owner, auth.GrantSpec{
		UserID: g.bobID, Role: string(auth.RoleReleaseManager), Scope: auth.ScopeEnvironment, AppID: g.app, EnvironmentID: g.dev,
	}); err != nil {
		t.Fatalf("environment grant: %v", err)
	}
	bob := g.principalOf(t, "bob@example.com", g.org)

	holds := func(scope auth.AccessScope, perm auth.Permission) bool {
		return slices.Contains(effective(t, bob, scope).Permissions, perm)
	}
	for name, c := range map[string]struct {
		scope auth.AccessScope
		perm  auth.Permission
		want  bool
	}{
		"the organisation has no plugin.edit":        {auth.AccessScope{}, auth.PluginEdit, false},
		"the granted app has plugin.edit":            {auth.AccessScope{AppID: g.app}, auth.PluginEdit, true},
		"its environment has plugin.edit":            {auth.AccessScope{AppID: g.app, Environment: "production"}, auth.PluginEdit, true},
		"another app has no plugin.edit":             {auth.AccessScope{AppID: blog}, auth.PluginEdit, false},
		"the environment grant promotes there":       {auth.AccessScope{AppID: g.app, Environment: "development"}, auth.ReleasePromote, true},
		"not to the other environment":               {auth.AccessScope{AppID: g.app, Environment: "production"}, auth.ReleasePromote, false},
		"nor to the app as a whole":                  {auth.AccessScope{AppID: g.app}, auth.ReleasePromote, false},
		"the organisation membership reads the apps": {auth.AccessScope{AppID: blog}, auth.AppRead, true},
	} {
		if got := holds(c.scope, c.perm); got != c.want {
			t.Errorf("%s: holds %s = %t, want %t", name, c.perm, got, c.want)
		}
	}
	set := effective(t, bob, auth.AccessScope{AppID: g.app})
	if !slices.IsSorted(set.Permissions) || len(slices.Compact(slices.Clone(set.Permissions))) != len(set.Permissions) {
		t.Errorf("the set is not sorted and without duplicates: %v", set.Permissions)
	}
	if want := g.version(t); set.Version != want {
		t.Errorf("version = %d, the organisation is at %d", set.Version, want)
	}

	if _, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{
		SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"plugin.*"}, AppID: g.app,
	}); err != nil {
		t.Fatalf("restriction: %v", err)
	}
	bob = g.principalOf(t, "bob@example.com", g.org)
	if holds(auth.AccessScope{AppID: g.app}, auth.PluginEdit) || holds(auth.AccessScope{AppID: g.app, Environment: "production"}, auth.PluginEdit) {
		t.Error("a restriction on the app left plugin.edit on it")
	}
	if !holds(auth.AccessScope{AppID: g.app}, auth.AppRead) {
		t.Error("a restriction of plugin.* removed app.read")
	}
	if set := effective(t, bob, auth.AccessScope{AppID: g.app}); set.Version <= effective(t, g.bob, auth.AccessScope{AppID: g.app}).Version {
		t.Errorf("version %d did not rise with the restriction", set.Version)
	}
	if _, err := bob.Effective(auth.AccessScope{Environment: "production"}); code(err) != plxerr.InvalidStructure {
		t.Errorf("an environment without an app = %v, want PLX-0002", err)
	}
}

// Verifies: GOV-007, GOV-006.
// Explain names the grants that allow a permission and the restrictions that
// remove it, by identifier, and an Owner whose floor keeps a permission a
// restriction names is told so.
func TestExplainNamesTheGrantAndTheRestriction_GOV_007(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	query := func(p auth.Principal, q auth.AccessQuery) auth.AccessDecision {
		t.Helper()
		d, err := g.svc.ExplainAccess(ctx, p, q)
		if err != nil {
			t.Fatalf("ExplainAccess(%+v): %v", q, err)
		}
		return d
	}
	edit := auth.AccessQuery{UserID: g.bobID, Permission: auth.PluginEdit}

	if d := query(g.owner, edit); d.Allowed || len(d.AllowingGrants) != 0 || len(d.RemovingRestrictions) != 0 {
		t.Errorf("a viewer's plugin.edit = %+v, want a denial that names no grant", d)
	}
	grant, err := g.svc.CreateGrant(ctx, g.owner, auth.GrantSpec{UserID: g.bobID, Role: string(auth.RoleDeveloper), Scope: auth.ScopeOrganization})
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}
	if d := query(g.owner, edit); !d.Allowed || !slices.Equal(d.AllowingGrants, []string{grant.ID}) || len(d.RemovingRestrictions) != 0 {
		t.Errorf("with the developer grant = %+v, want an allowance by grant %s", d, grant.ID)
	}
	restriction, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"plugin.edit"}})
	if err != nil {
		t.Fatalf("CreateRestriction: %v", err)
	}
	d := query(g.owner, edit)
	if d.Allowed || !slices.Equal(d.AllowingGrants, []string{grant.ID}) || !slices.Equal(d.RemovingRestrictions, []string{restriction.ID}) {
		t.Errorf("with the restriction = %+v, want a denial naming grant %s and restriction %s", d, grant.ID, restriction.ID)
	}
	if other := query(g.owner, auth.AccessQuery{UserID: g.bobID, Permission: auth.PluginEdit, Scope: auth.AccessScope{AppID: g.app}}); other.Allowed {
		t.Errorf("an organisation-wide restriction on the person left plugin.edit on an app: %+v", other)
	}

	// The Owner's floor survives a proposed restriction that names it.
	floor := query(g.owner, auth.AccessQuery{Permission: auth.MembersManage, Restriction: &auth.RestrictionSpec{
		SubjectKind: auth.SubjectOrganization, Patterns: []string{"members.manage"},
	}})
	if !floor.Allowed || !floor.OwnerFloor || len(floor.RemovingRestrictions) != 0 {
		t.Errorf("the Owner's members.manage under a proposed restriction = %+v, want it kept by the floor", floor)
	}
}

// Verifies: GOV-007.
// Simulating a proposed grant or restriction gives the answer the change
// would, and saves nothing: no grant, no restriction, no version, no audit
// entry.
func TestExplainSimulatesWithoutSaving_GOV_007(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	g.joinTeam(t, g.bobID, "viewer")
	before := g.version(t)
	changes := func() int {
		return g.auditCount(t, string(audit.RoleGranted)) + g.auditCount(t, string(audit.RestrictionCreated))
	}
	audits := changes()
	grants, err := g.svc.ListGrants(ctx, g.owner, auth.GrantFilter{}, storage.Cursor{}, 100)
	if err != nil {
		t.Fatalf("ListGrants: %v", err)
	}

	asked := auth.AccessQuery{UserID: g.bobID, Permission: auth.PluginEdit}
	granted := asked
	granted.Grant = &auth.ProposedGrant{UserID: g.bobID, Role: string(auth.RoleDeveloper), Scope: auth.ScopeOrganization}
	d, err := g.svc.ExplainAccess(ctx, g.owner, granted)
	if err != nil {
		t.Fatalf("simulate a grant: %v", err)
	}
	if !d.Allowed || !slices.Equal(d.AllowingGrants, []string{auth.ProposedID}) {
		t.Errorf("proposed grant = %+v, want an allowance by %q", d, auth.ProposedID)
	}
	// A grant to a team Bob belongs to reaches him; one to another team does not.
	teamGrant := asked
	teamGrant.Grant = &auth.ProposedGrant{TeamID: g.team, Role: string(auth.RoleDeveloper), Scope: auth.ScopeOrganization}
	if d, err := g.svc.ExplainAccess(ctx, g.owner, teamGrant); err != nil || !d.Allowed {
		t.Errorf("a proposed grant to Bob's team = %+v, %v", d, err)
	}
	otherTeam := asked
	otherTeam.Grant = &auth.ProposedGrant{TeamID: unknownTeam, Role: string(auth.RoleDeveloper), Scope: auth.ScopeOrganization}
	if d, err := g.svc.ExplainAccess(ctx, g.owner, otherTeam); err != nil || d.Allowed {
		t.Errorf("a proposed grant to a team Bob is not in = %+v, %v", d, err)
	}
	restricted := auth.AccessQuery{UserID: g.bobID, Permission: auth.AppRead, Restriction: &auth.RestrictionSpec{
		SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"app.*"},
	}}
	d, err = g.svc.ExplainAccess(ctx, g.owner, restricted)
	if err != nil {
		t.Fatalf("simulate a restriction: %v", err)
	}
	if d.Allowed || !slices.Equal(d.RemovingRestrictions, []string{auth.ProposedID}) {
		t.Errorf("proposed restriction = %+v, want a denial by %q", d, auth.ProposedID)
	}

	found, err := g.svc.ListRestrictions(ctx, g.owner, auth.RestrictionFilter{}, storage.Cursor{}, 100)
	if err != nil || len(found) != 0 {
		t.Errorf("restrictions after simulating = %v, %v; want none", found, err)
	}
	if after := g.version(t); after != before {
		t.Errorf("the permissions version moved from %d to %d by simulating", before, after)
	}
	if after := changes(); after != audits {
		t.Errorf("audit entries rose from %d to %d by simulating", audits, after)
	}
	if now, err := g.svc.ListGrants(ctx, g.owner, auth.GrantFilter{}, storage.Cursor{}, 100); err != nil || len(now) != len(grants) {
		t.Errorf("grants after simulating = %d, %v; want the %d there were", len(now), err, len(grants))
	}

	// A proposal that is not well formed is refused as it would be saved.
	bad := asked
	bad.Restriction = &auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"no.such.permission"}}
	if _, err := g.svc.ExplainAccess(ctx, g.owner, bad); code(err) != plxerr.RestrictionPatternInvalid {
		t.Errorf("a proposed restriction with an unknown pattern = %v, want PLX-8104", err)
	}
	both := granted
	both.Restriction = restricted.Restriction
	if _, err := g.svc.ExplainAccess(ctx, g.owner, both); code(err) != plxerr.InvalidStructure {
		t.Errorf("a grant and a restriction together = %v, want PLX-0002", err)
	}
}

// Verifies: GOV-007, SEC-103.
// Anyone may explain their own access; explaining another person's needs
// members.manage, and a person outside the organisation is not found.
func TestExplainOthersNeedsMembersManage_GOV_007(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	if d, err := g.svc.ExplainAccess(ctx, g.bob, auth.AccessQuery{Permission: auth.AppRead}); err != nil || !d.Allowed || len(d.AllowingGrants) == 0 {
		t.Errorf("a viewer explaining their own app.read = %+v, %v", d, err)
	}
	if d, err := g.svc.ExplainAccess(ctx, g.bob, auth.AccessQuery{UserID: g.bobID, Permission: auth.AppRead}); err != nil || !d.Allowed {
		t.Errorf("a viewer naming themselves = %+v, %v", d, err)
	}
	if _, err := g.svc.ExplainAccess(ctx, g.bob, auth.AccessQuery{UserID: g.owner.UserID, Permission: auth.AppRead}); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer explaining the Owner = %v, want denied", err)
	}
	if _, err := g.svc.ExplainAccess(ctx, g.bob, auth.AccessQuery{
		UserID: g.owner.UserID, Permission: auth.AppRead,
		Grant: &auth.ProposedGrant{UserID: g.owner.UserID, Role: string(auth.RoleViewer), Scope: auth.ScopeOrganization},
	}); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer simulating a grant for the Owner = %v, want denied", err)
	}
	if _, err := g.svc.ExplainAccess(ctx, g.owner, auth.AccessQuery{UserID: g.person(t, "eve@example.com"), Permission: auth.AppRead}); code(err) != plxerr.ResourceNotFound {
		t.Errorf("explaining someone outside the organisation = %v, want not found", err)
	}
	if _, err := g.svc.ExplainAccess(ctx, g.owner, auth.AccessQuery{UserID: g.bobID, Permission: "no.such.permission"}); err == nil {
		t.Error("explaining a permission that does not exist succeeded")
	}
}
