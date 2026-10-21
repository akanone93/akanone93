// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// addMember adds a person to the governance fixture's organisation with a
// direct membership and returns their user ID.
func (g *governance) addMember(t *testing.T, email string, role auth.Role) string {
	t.Helper()
	user := g.person(t, email)
	id, _ := g.ids.New()
	err := g.db.InTx(context.Background(), storage.Tenant{OrganizationID: g.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO memberships (id, organization_id, user_id, role) VALUES ($1, $2, $3, $4)`, id, g.org, user, string(role))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

// joinTeam makes a person a member of the fixture's team.
func (g *governance) joinTeam(t *testing.T, user, role string) {
	t.Helper()
	id, _ := g.ids.New()
	err := g.db.InTx(context.Background(), storage.Tenant{OrganizationID: g.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO memberships (id, organization_id, user_id, team_id, role) VALUES ($1, $2, $3, $4, $5)`, id, g.org, user, g.team, role)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// secondApp adds an app named blog and returns its ID.
func (g *governance) secondApp(t *testing.T) string {
	t.Helper()
	id, _ := g.ids.New()
	err := g.db.InTx(context.Background(), storage.Tenant{OrganizationID: g.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO apps (id, organization_id, key, name) VALUES ($1, $2, 'blog', 'Blog')`, id, g.org)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// auditCount counts the audit entries of an action in the organisation.
func (g *governance) auditCount(t *testing.T, action string) int {
	t.Helper()
	var n int
	err := g.db.InTx(context.Background(), storage.Tenant{OrganizationID: g.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = $1`, action).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// version reads the organisation's permissions version.
func (g *governance) version(t *testing.T) int64 {
	t.Helper()
	v, err := g.svc.PermissionsVersion(context.Background(), g.org)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// grantDeveloper gives a person the developer role across the organisation.
func (g *governance) grantDeveloper(t *testing.T, user string) {
	t.Helper()
	if _, err := g.svc.CreateGrant(context.Background(), g.owner, auth.GrantSpec{UserID: user, Role: string(auth.RoleDeveloper), Scope: auth.ScopeOrganization}); err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}
}

// Verifies: GOV-006, SEC-102.
// A restriction reaches a person by the organisation, by a team they belong
// to and by their own name, each removing the permission from them and from
// nobody it does not apply to.
func TestRestrictionsReachThePerson_GOV_006(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	cy := g.addMember(t, "cy@example.com", auth.RoleDeveloper)
	g.addMember(t, "dee@example.com", auth.RoleDeveloper)
	g.joinTeam(t, g.bobID, "viewer")
	g.grantDeveloper(t, g.bobID)
	editors := func() map[string]bool {
		out := map[string]bool{}
		for _, email := range []string{"bob@example.com", "cy@example.com", "dee@example.com"} {
			out[email] = g.principalOf(t, email, g.org).Holds(auth.PluginEdit)
		}
		return out
	}
	if got := editors(); !got["bob@example.com"] || !got["cy@example.com"] || !got["dee@example.com"] {
		t.Fatalf("before any restriction: %v", got)
	}
	team, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{SubjectKind: auth.SubjectTeam, SubjectID: g.team, Patterns: []string{"plugin.edit"}, Reason: "freeze web"})
	if err != nil {
		t.Fatalf("team restriction: %v", err)
	}
	if got := editors(); got["bob@example.com"] || !got["cy@example.com"] || !got["dee@example.com"] {
		t.Errorf("after the team restriction: %v", got)
	}
	if _, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: cy, Patterns: []string{"plugin.*"}}); err != nil {
		t.Fatalf("person restriction: %v", err)
	}
	if got := editors(); got["bob@example.com"] || got["cy@example.com"] || !got["dee@example.com"] {
		t.Errorf("after the person restriction: %v", got)
	}
	org, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{SubjectKind: auth.SubjectOrganization, Patterns: []string{"plugin.edit"}})
	if err != nil {
		t.Fatalf("organisation restriction: %v", err)
	}
	if got := editors(); got["bob@example.com"] || got["cy@example.com"] || got["dee@example.com"] {
		t.Errorf("after the organisation restriction: %v", got)
	}
	// The principal says which restriction removed what.
	bob := g.principalOf(t, "bob@example.com", g.org)
	ids := map[string][]auth.Permission{}
	for _, r := range bob.Restrictions {
		ids[r.ID] = r.Removed
	}
	if !slices.Equal(ids[team.ID], []auth.Permission{auth.PluginEdit}) || !slices.Equal(ids[org.ID], []auth.Permission{auth.PluginEdit}) || len(bob.Restrictions) != 2 {
		t.Errorf("Bob's applied restrictions = %+v", bob.Restrictions)
	}
	// Deleting the team's and the organisation's restrictions gives Bob the
	// permission back, as it gives nobody else more.
	for _, id := range []string{team.ID, org.ID} {
		if err := g.svc.DeleteRestriction(ctx, g.owner, id); err != nil {
			t.Fatalf("DeleteRestriction: %v", err)
		}
	}
	if got := editors(); !got["bob@example.com"] || got["cy@example.com"] || !got["dee@example.com"] {
		t.Errorf("after deleting two restrictions: %v", got)
	}
}

// Verifies: GOV-006, SEC-102.
// A restriction on one app removes an organisation-wide permission there and
// leaves it on the other app; a restriction on one environment leaves the
// rest of the app.
func TestRestrictionsHonourAppAndEnvironment_GOV_006(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	blog := g.secondApp(t)
	g.grantDeveloper(t, g.bobID)
	if _, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{
		SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"plugin.*"}, AppID: g.app,
	}); err != nil {
		t.Fatalf("app restriction: %v", err)
	}
	bob := g.principalOf(t, "bob@example.com", g.org)
	if err := bob.AuthorizeApp(auth.PluginEdit, g.app); code(err) != plxerr.PermissionDenied {
		t.Errorf("plugin.edit on the restricted app = %v, want denied", err)
	}
	if err := bob.AuthorizeApp(auth.PluginEdit, blog); err != nil {
		t.Errorf("plugin.edit on the other app = %v", err)
	}
	if err := bob.AuthorizeApp(auth.AppRead, g.app); err != nil {
		t.Errorf("app.read on the restricted app = %v; only plugin.* was restricted", err)
	}
	if _, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{
		SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"release.publish"}, AppID: blog, Environment: "production",
	}); code(err) != plxerr.ScopeInvalid {
		t.Errorf("an environment the app does not have = %v, want PLX-8102", err)
	}
	if _, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{
		SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"release.publish"}, AppID: g.app, Environment: "production",
	}); err != nil {
		t.Fatalf("environment restriction: %v", err)
	}
	bob = g.principalOf(t, "bob@example.com", g.org)
	if err := bob.AuthorizeEnvironment(auth.ReleasePublish, g.app, "production"); code(err) != plxerr.PermissionDenied {
		t.Errorf("publishing to production = %v, want denied", err)
	}
	if err := bob.AuthorizeEnvironment(auth.ReleasePublish, g.app, "development"); err != nil {
		t.Errorf("publishing to development = %v", err)
	}
	if err := bob.AuthorizeApp(auth.ReleasePublish, g.app); err != nil {
		t.Errorf("publishing as the app = %v; only production was restricted", err)
	}
}

// Verifies: GOV-006, SEC-102.
// An Owner keeps the owner floor under a restriction that names it, and a
// restriction on the organisation that names it is refused when it would
// leave no Owner able to manage; both refusals are audited.
func TestLastOwnerIsNeverLockedOut_GOV_006(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	ada := g.owner.UserID
	for _, c := range []struct {
		name string
		spec auth.RestrictionSpec
	}{
		{"the person", auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: ada, Patterns: []string{"members.manage"}}},
		{"the organisation", auth.RestrictionSpec{SubjectKind: auth.SubjectOrganization, Patterns: []string{"*"}}},
		{"an area", auth.RestrictionSpec{SubjectKind: auth.SubjectOrganization, Patterns: []string{"restrictions.*"}}},
	} {
		before := g.auditCount(t, "restriction.refused")
		if _, err := g.svc.CreateRestriction(ctx, g.owner, c.spec); code(err) != plxerr.RestrictionLocksOutOwner {
			t.Errorf("%s: err = %v, want PLX-8105", c.name, err)
		}
		if after := g.auditCount(t, "restriction.refused"); after != before+1 {
			t.Errorf("%s: refusals audited = %d, want %d", c.name, after, before+1)
		}
	}
	list, err := g.svc.ListRestrictions(ctx, g.owner, auth.RestrictionFilter{}, storage.Cursor{}, 10)
	if err != nil || len(list) != 0 {
		t.Errorf("restrictions after the refusals = %+v, %v", list, err)
	}
	// A second Owner who stays out of reach makes the same restriction on
	// the first one possible, and the floor still holds for them.
	grace := g.addMember(t, "grace@example.com", auth.RoleOwner)
	if _, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: ada, Patterns: []string{"members.manage"}}); err != nil {
		t.Fatalf("restricting one of two owners: %v", err)
	}
	if _, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: grace, Patterns: []string{"members.manage"}}); code(err) != plxerr.RestrictionLocksOutOwner {
		t.Errorf("restricting the other owner too = %v, want PLX-8105", err)
	}
	if err := g.principalOf(t, "ada@example.com", g.org).Authorize(auth.MembersManage); err != nil {
		t.Errorf("the restricted owner lost members.manage: %v", err)
	}
}

// Verifies: GOV-006, SEC-102.
// Nobody restricts what they do not hold: an Admin restricts what an Admin
// holds and is refused anything wider, and the refusal is audited.
func TestRestrictionCannotExceedTheCaller_GOV_006(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	g.addMember(t, "admin@example.com", auth.RoleAdmin)
	admin := g.principalOf(t, "admin@example.com", g.org)
	for _, patterns := range [][]string{{"*"}, {"members.manage"}, {"plugin.edit", "keys.manage"}, {"audit.*", "dsr.manage"}} {
		before := g.auditCount(t, "restriction.refused")
		_, err := g.svc.CreateRestriction(ctx, admin, auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: patterns})
		if code(err) != plxerr.RestrictionExceedsCaller {
			t.Errorf("%v: err = %v, want PLX-8106", patterns, err)
		}
		if after := g.auditCount(t, "restriction.refused"); after != before+1 {
			t.Errorf("%v: refusals audited = %d, want %d", patterns, after, before+1)
		}
	}
	if _, err := g.svc.CreateRestriction(ctx, admin, auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"plugin.*", "release.*"}}); err != nil {
		t.Errorf("an admin restricting what they hold: %v", err)
	}
	// On one app the measure is what the admin holds on that app.
	if _, err := g.svc.CreateRestriction(ctx, admin, auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"plugin.*"}, AppID: g.app}); err != nil {
		t.Errorf("an admin restricting what they hold on one app: %v", err)
	}
	if _, err := g.svc.CreateRestriction(ctx, admin, auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"*"}, AppID: g.app}); code(err) != plxerr.RestrictionExceedsCaller {
		t.Errorf("an admin restricting everything on one app = %v, want PLX-8106", err)
	}
}

// Verifies: GOV-006, SEC-102, SEC-103.
// Changing restrictions needs restrictions.manage and a step-up; the whole
// organisation also needs organization.manage; both apply to deleting.
func TestRestrictionChangeAuthorisation_GOV_006(t *testing.T) {
	t.Parallel()
	g := newGovernance(t, func(o *auth.Options) { o.CustomRolesEnabled = func() bool { return true } })
	ctx := context.Background()
	if _, err := g.svc.CreateRole(ctx, g.owner, "restrictor", "Restrictor", "", []string{"restrictions.manage", "plugin.edit", "organization.read"}); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if _, err := g.svc.CreateGrant(ctx, g.owner, auth.GrantSpec{UserID: g.bobID, Role: "restrictor", Scope: auth.ScopeOrganization}); err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}
	bob := g.principalOf(t, "bob@example.com", g.org)
	team := auth.RestrictionSpec{SubjectKind: auth.SubjectTeam, SubjectID: g.team, Patterns: []string{"plugin.edit"}}
	whole := auth.RestrictionSpec{SubjectKind: auth.SubjectOrganization, Patterns: []string{"plugin.edit"}}
	if _, err := g.svc.CreateRestriction(ctx, bob, whole); code(err) != plxerr.PermissionDenied {
		t.Errorf("the whole organisation without organization.manage = %v, want denied", err)
	}
	// Without a second factor in the session.
	noFactor := bob
	noFactor.SecondFactor = false
	if _, err := g.svc.CreateRestriction(ctx, noFactor, team); code(err) != plxerr.MultiFactorRequired {
		t.Errorf("without a second factor = %v, want PLX-8011", err)
	}
	if _, err := g.svc.ListRestrictions(ctx, noFactor, auth.RestrictionFilter{}, storage.Cursor{}, 10); code(err) != plxerr.MultiFactorRequired {
		t.Errorf("listing without a second factor = %v", err)
	}
	// Someone without the permission at all.
	viewer := g.principalOf(t, "bob@example.com", g.org)
	viewer.Permissions = nil
	if _, err := g.svc.CreateRestriction(ctx, viewer, team); code(err) != plxerr.PermissionDenied {
		t.Errorf("without restrictions.manage = %v, want denied", err)
	}
	created, err := g.svc.CreateRestriction(ctx, bob, team)
	if err != nil {
		t.Fatalf("a team restriction by a restrictor: %v", err)
	}
	whole1, err := g.svc.CreateRestriction(ctx, g.owner, whole)
	if err != nil {
		t.Fatalf("the owner restricting the organisation: %v", err)
	}
	if err := g.svc.DeleteRestriction(ctx, bob, whole1.ID); code(err) != plxerr.PermissionDenied {
		t.Errorf("deleting the organisation's restriction without organization.manage = %v, want denied", err)
	}
	if err := g.svc.DeleteRestriction(ctx, bob, created.ID); err != nil {
		t.Errorf("deleting a team restriction: %v", err)
	}
	if err := g.svc.DeleteRestriction(ctx, bob, created.ID); code(err) != plxerr.ResourceNotFound {
		t.Errorf("deleting it twice = %v, want not found", err)
	}
	if err := g.svc.DeleteRestriction(ctx, g.owner, "not-an-id"); code(err) != plxerr.InvalidFormat {
		t.Errorf("deleting a malformed id = %v", err)
	}
}

// Verifies: GOV-006.
// A restriction is validated before it is stored: the patterns against the
// catalogue (PLX-8104), an environment needs an app, the subject must exist
// in the organisation, and the app too.
func TestRestrictionValidation_GOV_006(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	other, _ := g.ids.New()
	for _, c := range []struct {
		name string
		spec auth.RestrictionSpec
		want plxerr.Code
	}{
		{"no pattern", auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID}, plxerr.RestrictionPatternInvalid},
		{"unknown permission", auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"plugin.nonsense"}}, plxerr.RestrictionPatternInvalid},
		{"unknown area", auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"nothing.*"}}, plxerr.RestrictionPatternInvalid},
		{"environment without an app", auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"*"}, Environment: "production"}, plxerr.RestrictionPatternInvalid},
		{"malformed environment", auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"*"}, AppID: g.app, Environment: "Prod!"}, plxerr.RestrictionPatternInvalid},
		{"unknown subject kind", auth.RestrictionSpec{SubjectKind: "group", SubjectID: g.bobID, Patterns: []string{"*"}}, plxerr.InvalidStructure},
		{"organisation with a subject", auth.RestrictionSpec{SubjectKind: auth.SubjectOrganization, SubjectID: g.bobID, Patterns: []string{"plugin.*"}}, plxerr.InvalidStructure},
		{"malformed subject", auth.RestrictionSpec{SubjectKind: auth.SubjectTeam, SubjectID: "x", Patterns: []string{"plugin.*"}}, plxerr.InvalidFormat},
		{"unknown team", auth.RestrictionSpec{SubjectKind: auth.SubjectTeam, SubjectID: other, Patterns: []string{"plugin.*"}}, plxerr.ResourceNotFound},
		{"stranger", auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: other, Patterns: []string{"plugin.*"}}, plxerr.ResourceNotFound},
		{"unknown app", auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"plugin.*"}, AppID: other}, plxerr.ScopeInvalid},
		{"malformed app", auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"plugin.*"}, AppID: "x"}, plxerr.ScopeInvalid},
	} {
		if _, err := g.svc.CreateRestriction(ctx, g.owner, c.spec); code(err) != c.want {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	if n := g.auditCount(t, "restriction.created"); n != 0 {
		t.Errorf("%d restrictions were audited as created; none should have been", n)
	}
}

// Verifies: GOV-006, LIM-001.
// The counts come from the limits registry: restrictions per organisation
// and per subject, and patterns per restriction (PLX-8107).
func TestRestrictionLimits_GOV_006(t *testing.T) {
	t.Parallel()
	set := limits.Defaults()
	for k, v := range map[limits.Key]int64{limits.AuthRestrictionsPerOrg: 3, limits.AuthRestrictionsPerSubject: 2, limits.AuthRestrictionPatterns: 2} {
		var err error
		if set, err = set.Tighten(k, limits.ScopeInstallation, v); err != nil {
			t.Fatal(err)
		}
	}
	g := newGovernance(t, func(o *auth.Options) { o.Limits = set })
	ctx := context.Background()
	cy := g.addMember(t, "cy@example.com", auth.RoleViewer)
	restrictUser := func(user string, patterns ...string) error {
		_, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: user, Patterns: patterns})
		return err
	}
	if err := restrictUser(g.bobID, "plugin.edit", "plugin.read", "app.read"); code(err) != plxerr.RestrictionLimitReached {
		t.Errorf("three patterns over a limit of two = %v, want PLX-8107", err)
	}
	if err := restrictUser(g.bobID, "plugin.edit"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := restrictUser(g.bobID, "plugin.read"); err != nil {
		t.Fatalf("second: %v", err)
	}
	if err := restrictUser(g.bobID, "app.read"); code(err) != plxerr.RestrictionLimitReached {
		t.Errorf("a third for one subject = %v, want PLX-8107", err)
	}
	if err := restrictUser(cy, "plugin.edit"); err != nil {
		t.Fatalf("another subject: %v", err)
	}
	if err := restrictUser(cy, "plugin.read"); code(err) != plxerr.RestrictionLimitReached {
		t.Errorf("a fourth in the organisation = %v, want PLX-8107", err)
	}
	list, err := g.svc.ListRestrictions(ctx, g.owner, auth.RestrictionFilter{SubjectID: g.bobID}, storage.Cursor{}, 10)
	if err != nil || len(list) != 2 {
		t.Errorf("Bob's restrictions = %+v, %v", list, err)
	}
	if err := g.svc.DeleteRestriction(ctx, g.owner, list[0].ID); err != nil {
		t.Fatalf("DeleteRestriction: %v", err)
	}
	if err := restrictUser(cy, "plugin.read"); err != nil {
		t.Errorf("after deleting one: %v", err)
	}
}

// Verifies: GOV-006, GOV-007.
// The organisation's permissions version rises with every change to grants,
// roles, team membership and restrictions, in the same transaction, and a
// refused change leaves it alone.
func TestPermissionsVersionBumps_GOV_006(t *testing.T) {
	t.Parallel()
	g := newGovernance(t, func(o *auth.Options) { o.CustomRolesEnabled = func() bool { return true } })
	ctx := context.Background()
	rose := func(name string, change func() error) {
		t.Helper()
		before := g.version(t)
		if err := change(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if after := g.version(t); after <= before {
			t.Errorf("%s: version %d -> %d, want it to rise", name, before, after)
		}
	}
	var grant auth.RoleGrantRecord
	rose("a grant", func() (err error) {
		grant, err = g.svc.CreateGrant(ctx, g.owner, auth.GrantSpec{UserID: g.bobID, Role: "developer", Scope: auth.ScopeOrganization})
		return err
	})
	rose("a grant's role", func() (err error) {
		_, err = g.svc.UpdateGrant(ctx, g.owner, grant.ID, "viewer")
		return err
	})
	rose("a grant removed", func() error { return g.svc.DeleteGrant(ctx, g.owner, grant.ID) })
	rose("a role", func() (err error) {
		_, err = g.svc.CreateRole(ctx, g.owner, "readers", "Readers", "", []string{"app.read"})
		return err
	})
	rose("a role changed", func() (err error) {
		_, err = g.svc.UpdateRole(ctx, g.owner, "readers", "Readers", "", []string{"app.read", "plugin.read"})
		return err
	})
	rose("a role removed", func() error { return g.svc.DeleteRole(ctx, g.owner, "readers") })
	rose("team membership", func() error { g.joinTeam(t, g.bobID, "viewer"); return nil })
	var r auth.Restriction
	rose("a restriction", func() (err error) {
		r, err = g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{SubjectKind: auth.SubjectTeam, SubjectID: g.team, Patterns: []string{"plugin.*"}})
		return err
	})
	rose("a restriction removed", func() error { return g.svc.DeleteRestriction(ctx, g.owner, r.ID) })
	before := g.version(t)
	if _, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{SubjectKind: auth.SubjectOrganization, Patterns: []string{"*"}}); err == nil {
		t.Fatal("the lockout was not refused")
	}
	if after := g.version(t); after != before {
		t.Errorf("a refused restriction moved the version: %d -> %d", before, after)
	}
}

// Verifies: GOV-006, SEC-102.
// A workload identity is bound by the organisation's restrictions: they are
// loaded for it although it has no person.
func TestRestrictionBindsAWorkloadIdentity_GOV_006(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	ci := auth.Identity{Kind: auth.KindCI, ID: "t", OrganizationID: g.org, Scopes: []auth.Permission{auth.PluginRead, auth.ReleasePublish}, SecondFactor: true}
	p, err := g.svc.Resolve(ctx, ci, g.org)
	if err != nil || !p.Holds(auth.ReleasePublish) {
		t.Fatalf("before: %v, %v", p.Permissions, err)
	}
	if _, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{SubjectKind: auth.SubjectOrganization, Patterns: []string{"release.*"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"plugin.read"}}); err != nil {
		t.Fatal(err)
	}
	p, err = g.svc.Resolve(ctx, ci, g.org)
	if err != nil || p.Holds(auth.ReleasePublish) || !p.Holds(auth.PluginRead) {
		t.Errorf("after: %v, %v; the organisation's restriction applies and Bob's does not", p.Permissions, err)
	}
}

// Verifies: GOV-006.
// Restrictions are listed in identifier order with their subject, patterns,
// scope, reason and author, and filtered by subject and app.
func TestListRestrictions_GOV_006(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	if _, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{
		SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"release.*", "plugin.edit", "plugin.edit"},
		AppID: g.app, Environment: "production", Reason: " release freeze ",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{SubjectKind: auth.SubjectTeam, SubjectID: g.team, Patterns: []string{"app.read"}}); err != nil {
		t.Fatal(err)
	}
	all, err := g.svc.ListRestrictions(ctx, g.owner, auth.RestrictionFilter{}, storage.Cursor{}, 10)
	if err != nil || len(all) != 2 || all[0].ID >= all[1].ID {
		t.Fatalf("all = %+v, %v", all, err)
	}
	first := all[0]
	if first.SubjectKind != auth.SubjectUser || first.SubjectID != g.bobID || first.AppID != g.app || first.Environment != "production" ||
		first.Reason != "release freeze" || first.CreatedBy != g.owner.UserID || !slices.Equal(first.Patterns, []string{"plugin.edit", "release.*"}) || first.CreatedAt.IsZero() {
		t.Errorf("first = %+v", first)
	}
	byApp, err := g.svc.ListRestrictions(ctx, g.owner, auth.RestrictionFilter{AppID: g.app}, storage.Cursor{}, 10)
	if err != nil || len(byApp) != 1 || byApp[0].ID != first.ID {
		t.Errorf("by app = %+v, %v", byApp, err)
	}
	bySubject, err := g.svc.ListRestrictions(ctx, g.owner, auth.RestrictionFilter{SubjectID: g.team}, storage.Cursor{}, 10)
	if err != nil || len(bySubject) != 1 || bySubject[0].SubjectKind != auth.SubjectTeam {
		t.Errorf("by subject = %+v, %v", bySubject, err)
	}
	if _, err := g.svc.ListRestrictions(ctx, g.owner, auth.RestrictionFilter{AppID: "x"}, storage.Cursor{}, 10); code(err) != plxerr.InvalidFormat {
		t.Errorf("a malformed app filter = %v", err)
	}
	for _, email := range []string{"bob@example.com"} {
		if _, err := g.svc.ListRestrictions(ctx, g.principalOf(t, email, g.org), auth.RestrictionFilter{}, storage.Cursor{}, 10); code(err) != plxerr.PermissionDenied {
			t.Errorf("a viewer listing = %v, want denied", err)
		}
	}
	// Every change is audited.
	if n := g.auditCount(t, "restriction.created"); n != 2 {
		t.Errorf("created entries = %d, want 2", n)
	}
	if err := g.svc.DeleteRestriction(ctx, g.owner, first.ID); err != nil {
		t.Fatal(err)
	}
	if n := g.auditCount(t, "restriction.deleted"); n != 1 {
		t.Errorf("deleted entries = %d, want 1", n)
	}
}

// Verifies: GOV-006, LIM-001.
// An organisation's own override of a restriction limit can only lower it.
func TestRestrictionLimitOverride_GOV_006(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	override := func(value int64) {
		t.Helper()
		err := g.db.InTx(ctx, storage.Tenant{OrganizationID: g.org}, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO limit_overrides (organization_id, scope, scope_id, limit_key, value)
				VALUES ($1, 'organization', $1, 'auth.restrictionsPerOrg', $2)
				ON CONFLICT (scope, scope_id, limit_key) DO UPDATE SET value = $2`, g.org, value)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	spec := auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: g.bobID, Patterns: []string{"plugin.edit"}}
	override(1)
	if _, err := g.svc.CreateRestriction(ctx, g.owner, spec); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := g.svc.CreateRestriction(ctx, g.owner, spec); code(err) != plxerr.RestrictionLimitReached {
		t.Errorf("second under an override of one = %v, want PLX-8107", err)
	}
	// An override above the default is not a tightening and has no effect.
	override(100000)
	if _, err := g.svc.CreateRestriction(ctx, g.owner, spec); err != nil {
		t.Errorf("second under a raised override = %v", err)
	}
}

// Verifies: GOV-006.
// Only an Owner restricts the whole organisation, though an Admin holds
// organization.manage; and a restricted Admin cannot lift a restriction that
// applies to them, which is refused and audited, while an Owner can.
func TestOnlyAnOwnerRestrictsTheOrganisationOrLiftsOwnRestriction_GOV_006(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	adminID := g.addMember(t, "admin@example.com", auth.RoleAdmin)
	admin := g.principalOf(t, "admin@example.com", g.org)
	whole := auth.RestrictionSpec{SubjectKind: auth.SubjectOrganization, Patterns: []string{"plugin.edit"}}
	if _, err := g.svc.CreateRestriction(ctx, admin, whole); code(err) != plxerr.PermissionDenied {
		t.Errorf("an Admin restricting the organisation = %v, want denied", err)
	}
	own, err := g.svc.CreateRestriction(ctx, g.owner, auth.RestrictionSpec{SubjectKind: auth.SubjectUser, SubjectID: adminID, Patterns: []string{"plugin.edit"}})
	if err != nil {
		t.Fatalf("the owner restricting the Admin: %v", err)
	}
	refused := g.auditCount(t, "restriction.refused")
	if err := g.svc.DeleteRestriction(ctx, g.principalOf(t, "admin@example.com", g.org), own.ID); code(err) != plxerr.RestrictionExceedsCaller {
		t.Errorf("the Admin lifting their own restriction = %v, want PLX-8106", err)
	}
	if got := g.auditCount(t, "restriction.refused"); got != refused+1 {
		t.Errorf("refusals audited = %d, want %d", got, refused+1)
	}
	if err := g.svc.DeleteRestriction(ctx, g.owner, own.ID); err != nil {
		t.Errorf("the owner lifting it: %v", err)
	}
}
