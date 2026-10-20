// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// governance is an organisation with an owner, a second person, an app with
// a development and a production environment, and a team.
type governance struct {
	*fixture
	org, app, dev, prod, team string
	owner, bob                auth.Principal
	bobID                     string
}

// principalOf resolves a person's principal in the organisation. The
// session counts as having presented a second factor, which the roles and
// grants permissions require.
func (f *fixture) principalOf(t *testing.T, email, org string) auth.Principal {
	t.Helper()
	p, err := f.svc.Resolve(context.Background(), f.signIn(t, email), org)
	if err != nil {
		t.Fatalf("Resolve(%s): %v", email, err)
	}
	p.SecondFactor = true
	p.Identity = p.WithStepUp(0)
	return p
}

func newGovernance(t *testing.T, configure ...func(*auth.Options)) *governance {
	t.Helper()
	f := newFixtureWith(t, nil, failedSignIns, configure...)
	ctx := context.Background()
	ada := f.person(t, "ada@example.com")
	bob := f.person(t, "bob@example.com")
	g := &governance{fixture: f, bobID: bob}
	g.org = f.organisation(t, "acme", map[string]auth.Role{ada: auth.RoleOwner, bob: auth.RoleViewer})
	g.app, _ = f.ids.New()
	g.dev, _ = f.ids.New()
	g.prod, _ = f.ids.New()
	g.team, _ = f.ids.New()
	if err := f.db.InTx(ctx, storage.Tenant{OrganizationID: g.org}, func(ctx context.Context, tx pgx.Tx) error {
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO apps (id, organization_id, key, name) VALUES ($1, $2, 'shop', 'Shop')`, []any{g.app, g.org}},
			{`INSERT INTO environments (id, organization_id, app_id, key, name) VALUES ($1, $2, $3, 'development', 'Development')`, []any{g.dev, g.org, g.app}},
			{`INSERT INTO environments (id, organization_id, app_id, key, name) VALUES ($1, $2, $3, 'production', 'Production')`, []any{g.prod, g.org, g.app}},
			{`INSERT INTO teams (id, organization_id, key, name) VALUES ($1, $2, 'web', 'Web')`, []any{g.team, g.org}},
		} {
			if _, err := tx.Exec(ctx, q.sql, q.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	g.owner = f.principalOf(t, "ada@example.com", g.org)
	g.bob = f.principalOf(t, "bob@example.com", g.org)
	return g
}

// Verifies: GOV-002, GOV-003, QA-008.
// A grant on one environment gives that environment's permissions and
// nothing else: not the other environment, not the app as a whole.
func TestGrantOnAnEnvironment_GOV_003(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	if _, err := g.svc.CreateGrant(ctx, g.owner, auth.GrantSpec{
		UserID: g.bobID, Role: string(auth.RoleReleaseManager), Scope: auth.ScopeEnvironment, AppID: g.app, EnvironmentID: g.dev,
	}); err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}
	bob := g.principalOf(t, "bob@example.com", g.org)
	if err := bob.AuthorizeEnvironment(auth.ReleasePublish, g.app, "development"); err != nil {
		t.Errorf("publishing to development: %v", err)
	}
	if err := bob.AuthorizeEnvironment(auth.ReleasePublish, g.app, "production"); code(err) != plxerr.PermissionDenied {
		t.Errorf("publishing to production: %v", err)
	}
	if err := bob.AuthorizeApp(auth.ReleasePublish, g.app); code(err) != plxerr.PermissionDenied {
		t.Errorf("publishing as the app: %v", err)
	}
	if err := bob.AuthorizeSomeEnvironment(auth.ReleasePublish, g.app); err != nil {
		t.Errorf("publishing somewhere: %v", err)
	}
	// The grant carries only what can be limited to an environment: the
	// release manager's reading permissions stay with the viewer role.
	if bob.HoldsOnEnvironment(auth.PluginEdit, g.app, "development") || bob.HoldsOnEnvironment(auth.AuditRead, g.app, "development") {
		t.Errorf("an environment grant carried more than environment permissions: %v", bob.EnvPermissions)
	}
	// Without the second factor, the step-up permission is refused for what
	// it is, not for the scope.
	bob.SecondFactor = false
	if err := bob.AuthorizeEnvironment(auth.ReleasePublish, g.app, "development"); code(err) != plxerr.MultiFactorRequired {
		t.Errorf("publishing without a second factor: %v", err)
	}
}

// Verifies: GOV-002, GOV-003.
// A custom role with release.publish:production holds the permission on
// production alone, wherever the role is granted.
func TestScopedPermissionInACustomRole_GOV_003(t *testing.T) {
	t.Parallel()
	g := newGovernance(t, func(o *auth.Options) { o.CustomRolesEnabled = func() bool { return true } })
	ctx := context.Background()
	role, err := g.svc.CreateRole(ctx, g.owner, "prod-publisher", "Production publisher", "Publishes to production only", []string{
		"release.publish:production", "app.read", "release.publish:production",
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if want := []string{"app.read", "release.publish:production"}; len(role.Permissions) != 2 || role.Permissions[0] != want[0] || role.Permissions[1] != want[1] {
		t.Errorf("permissions = %v, want %v", role.Permissions, want)
	}
	// On the organisation, the role covers production of every app.
	if _, err := g.svc.CreateGrant(ctx, g.owner, auth.GrantSpec{UserID: g.bobID, Role: "prod-publisher", Scope: auth.ScopeOrganization}); err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}
	bob := g.principalOf(t, "bob@example.com", g.org)
	if err := bob.AuthorizeEnvironment(auth.ReleasePublish, g.app, "production"); err != nil {
		t.Errorf("publishing to production: %v", err)
	}
	if err := bob.AuthorizeEnvironment(auth.ReleasePublish, g.app, "development"); code(err) != plxerr.PermissionDenied {
		t.Errorf("publishing to development: %v", err)
	}
	if bob.Holds(auth.ReleasePublish) {
		t.Error("the scoped permission was held across the organisation")
	}
}

// Verifies: GOV-002.
// Custom roles are composed of catalogue permissions the creator holds,
// and are an enterprise feature.
func TestCustomRoles_GOV_002(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	off := newGovernance(t)
	if _, err := off.svc.CreateRole(ctx, off.owner, "auditors", "Auditors", "", []string{"audit.read"}); code(err) != plxerr.EnterpriseFeatureNotLicensed {
		t.Errorf("custom roles are on by default: %v", err)
	}

	g := newGovernance(t, func(o *auth.Options) { o.CustomRolesEnabled = func() bool { return true } })
	cases := []struct {
		name  string
		key   string
		perms []string
		want  plxerr.Code
	}{
		{"unknown permission", "ghost", []string{"app.read", "ghosts.manage"}, plxerr.PermissionNotInCatalogue},
		{"environment on a permission that cannot have one", "bad-scope", []string{"audit.read:production"}, plxerr.ScopeInvalid},
		{"a malformed environment", "bad-env", []string{"release.publish:Prod uction"}, plxerr.ScopeInvalid},
		{"no permissions", "empty", nil, plxerr.InvalidStructure},
		{"a predefined key", "viewer", []string{"app.read"}, plxerr.ResourceExists},
		{"a bad key", "Not A Key", []string{"app.read"}, plxerr.InvalidFormat},
	}
	for _, c := range cases {
		if _, err := g.svc.CreateRole(ctx, g.owner, c.key, "Name", "", c.perms); code(err) != c.want {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}

	role, err := g.svc.CreateRole(ctx, g.owner, "reader-plus", "Reader plus", "Reads and manages roles", []string{"app.read", "roles.manage", "members.manage"})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if _, err := g.svc.CreateRole(ctx, g.owner, "reader-plus", "Again", "", []string{"app.read"}); code(err) != plxerr.ResourceExists {
		t.Errorf("a duplicate key: %v", err)
	}

	// A person holding roles.manage may not compose a role of more than they hold.
	if _, err := g.svc.CreateGrant(ctx, g.owner, auth.GrantSpec{UserID: g.bobID, Role: role.Key, Scope: auth.ScopeOrganization}); err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}
	bob := g.principalOf(t, "bob@example.com", g.org)
	if _, err := g.svc.CreateRole(ctx, bob, "sneaky", "Sneaky", "", []string{"app.read", "keys.manage"}); code(err) != plxerr.PermissionDenied {
		t.Errorf("a role beyond its creator: %v", err)
	}
	if _, err := g.svc.UpdateRole(ctx, bob, role.Key, "Reader plus", "", []string{"app.read", "keys.manage"}); code(err) != plxerr.PermissionDenied {
		t.Errorf("updating a role beyond its creator: %v", err)
	}
	if _, err := g.svc.CreateGrant(ctx, bob, auth.GrantSpec{UserID: g.bobID, Role: string(auth.RoleOwner), Scope: auth.ScopeOrganization}); code(err) != plxerr.PermissionDenied {
		t.Errorf("granting the owner role without holding it: %v", err)
	}

	updated, err := g.svc.UpdateRole(ctx, g.owner, role.Key, "Reader", "Only reads", []string{"app.read", "members.manage"})
	if err != nil || updated.Name != "Reader" || len(updated.Permissions) != 2 {
		t.Errorf("UpdateRole = %+v, %v", updated, err)
	}
	if _, err := g.svc.UpdateRole(ctx, g.owner, "no-such-role", "X", "", []string{"app.read"}); code(err) != plxerr.RoleUnknown {
		t.Errorf("updating an unknown role: %v", err)
	}
	if _, err := g.svc.UpdateRole(ctx, g.owner, "viewer", "X", "", []string{"app.read"}); code(err) != plxerr.PreconditionFailed {
		t.Errorf("updating a predefined role: %v", err)
	}

	roles, err := g.svc.ListRoles(ctx, g.owner, "", 50)
	if err != nil || len(roles) != 11 {
		t.Fatalf("ListRoles = %d roles, %v", len(roles), err)
	}
	for i := 1; i < len(roles); i++ {
		if roles[i-1].Key >= roles[i].Key {
			t.Errorf("roles are not in key order: %s before %s", roles[i-1].Key, roles[i].Key)
		}
	}
	page, err := g.svc.ListRoles(ctx, g.owner, roles[4].Key, 3)
	if err != nil || len(page) != 3 || page[0].Key != roles[5].Key {
		t.Errorf("ListRoles after %s = %v, %v", roles[4].Key, page, err)
	}
	if r, err := g.svc.GetRole(ctx, g.owner, "viewer"); err != nil || !r.Predefined || len(r.Permissions) == 0 {
		t.Errorf("GetRole(viewer) = %+v, %v", r, err)
	}
	if _, err := g.svc.GetRole(ctx, g.owner, "no-such-role"); code(err) != plxerr.RoleUnknown {
		t.Errorf("GetRole of an unknown role: %v", err)
	}

	// A role in use is not deleted; once its grants are gone it is.
	if err := g.svc.DeleteRole(ctx, g.owner, role.Key); code(err) != plxerr.PreconditionFailed {
		t.Errorf("deleting a role in use: %v", err)
	}
	grants, err := g.svc.ListGrants(ctx, g.owner, auth.GrantFilter{UserID: g.bobID}, storage.Cursor{}, 10)
	if err != nil || len(grants) != 1 {
		t.Fatalf("ListGrants = %v, %v", grants, err)
	}
	if err := g.svc.DeleteGrant(ctx, g.owner, grants[0].ID); err != nil {
		t.Fatalf("DeleteGrant: %v", err)
	}
	if err := g.svc.DeleteRole(ctx, g.owner, role.Key); err != nil {
		t.Errorf("DeleteRole: %v", err)
	}
	if err := g.svc.DeleteRole(ctx, g.owner, role.Key); code(err) != plxerr.RoleUnknown {
		t.Errorf("deleting a deleted role: %v", err)
	}
	if err := g.svc.DeleteRole(ctx, g.owner, "owner"); code(err) != plxerr.PreconditionFailed {
		t.Errorf("deleting a predefined role: %v", err)
	}

	// A person who does not manage roles may not touch them.
	if _, err := g.svc.CreateRole(ctx, g.bob, "x-role", "X", "", []string{"app.read"}); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer created a role: %v", err)
	}
}

// Verifies: GOV-003.
// A grant names a role that exists (PLX-8100) at a scope that exists
// (PLX-8102), for someone in the organisation, once.
func TestGrantValidation_GOV_003(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	otherApp, _ := g.ids.New()
	otherEnv, _ := g.ids.New()
	if err := g.db.InTx(ctx, storage.Tenant{OrganizationID: g.org}, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO apps (id, organization_id, key, name) VALUES ($1, $2, 'bank', 'Bank')`, otherApp, g.org); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO environments (id, organization_id, app_id, key, name) VALUES ($1, $2, $3, 'production', 'Production')`, otherEnv, g.org, otherApp)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	stranger := g.person(t, "eve@example.com")
	unknown, _ := g.ids.New()
	cases := []struct {
		name string
		spec auth.GrantSpec
		want plxerr.Code
	}{
		{"unknown role", auth.GrantSpec{UserID: g.bobID, Role: "wizard", Scope: auth.ScopeOrganization}, plxerr.RoleUnknown},
		{"unknown scope", auth.GrantSpec{UserID: g.bobID, Role: "viewer", Scope: "galaxy"}, plxerr.ScopeInvalid},
		{"organisation scope naming an app", auth.GrantSpec{UserID: g.bobID, Role: "viewer", Scope: auth.ScopeOrganization, AppID: g.app}, plxerr.ScopeInvalid},
		{"app scope without an app", auth.GrantSpec{UserID: g.bobID, Role: "viewer", Scope: auth.ScopeApp}, plxerr.ScopeInvalid},
		{"app scope naming an environment", auth.GrantSpec{UserID: g.bobID, Role: "viewer", Scope: auth.ScopeApp, AppID: g.app, EnvironmentID: g.dev}, plxerr.ScopeInvalid},
		{"environment scope without an environment", auth.GrantSpec{UserID: g.bobID, Role: "viewer", Scope: auth.ScopeEnvironment, AppID: g.app}, plxerr.ScopeInvalid},
		{"an unknown app", auth.GrantSpec{UserID: g.bobID, Role: "viewer", Scope: auth.ScopeApp, AppID: unknown}, plxerr.ScopeInvalid},
		{"an environment of another app", auth.GrantSpec{UserID: g.bobID, Role: "viewer", Scope: auth.ScopeEnvironment, AppID: g.app, EnvironmentID: otherEnv}, plxerr.ScopeInvalid},
		{"no holder", auth.GrantSpec{Role: "viewer", Scope: auth.ScopeOrganization}, plxerr.InvalidStructure},
		{"two holders", auth.GrantSpec{UserID: g.bobID, TeamID: g.team, Role: "viewer", Scope: auth.ScopeOrganization}, plxerr.InvalidStructure},
		{"a stranger", auth.GrantSpec{UserID: stranger, Role: "viewer", Scope: auth.ScopeOrganization}, plxerr.ResourceNotFound},
		{"an unknown team", auth.GrantSpec{TeamID: unknown, Role: "viewer", Scope: auth.ScopeOrganization}, plxerr.ResourceNotFound},
	}
	for _, c := range cases {
		if _, err := g.svc.CreateGrant(ctx, g.owner, c.spec); code(err) != c.want {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	spec := auth.GrantSpec{UserID: g.bobID, Role: "developer", Scope: auth.ScopeApp, AppID: g.app}
	made, err := g.svc.CreateGrant(ctx, g.owner, spec)
	if err != nil || made.Scope != auth.ScopeApp || made.AppID != g.app {
		t.Fatalf("CreateGrant = %+v, %v", made, err)
	}
	if _, err := g.svc.CreateGrant(ctx, g.owner, spec); code(err) != plxerr.ResourceExists {
		t.Errorf("a repeated grant: %v", err)
	}
	if _, err := g.svc.CreateGrant(ctx, g.bob, spec); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer made a grant: %v", err)
	}
	changed, err := g.svc.UpdateGrant(ctx, g.owner, made.ID, "viewer")
	if err != nil || changed.Role != "viewer" {
		t.Errorf("UpdateGrant = %+v, %v", changed, err)
	}
	if _, err := g.svc.UpdateGrant(ctx, g.owner, made.ID, "wizard"); code(err) != plxerr.RoleUnknown {
		t.Errorf("UpdateGrant to an unknown role: %v", err)
	}
	if _, err := g.svc.UpdateGrant(ctx, g.owner, unknown, "viewer"); code(err) != plxerr.ResourceNotFound {
		t.Errorf("UpdateGrant of an unknown grant: %v", err)
	}
	byApp, err := g.svc.ListGrants(ctx, g.owner, auth.GrantFilter{AppID: g.app}, storage.Cursor{}, 10)
	if err != nil || len(byApp) != 1 {
		t.Errorf("ListGrants by app = %v, %v", byApp, err)
	}
	none, err := g.svc.ListGrants(ctx, g.owner, auth.GrantFilter{AppID: otherApp}, storage.Cursor{}, 10)
	if err != nil || len(none) != 0 {
		t.Errorf("ListGrants by another app = %v, %v", none, err)
	}
	if err := g.svc.DeleteGrant(ctx, g.owner, made.ID); err != nil {
		t.Errorf("DeleteGrant: %v", err)
	}
	if err := g.svc.DeleteGrant(ctx, g.owner, made.ID); code(err) != plxerr.ResourceNotFound {
		t.Errorf("deleting a deleted grant: %v", err)
	}
}

// Verifies: GOV-002, GOV-003.
// A grant to a team reaches every member of the team, and ends with the
// membership.
func TestTeamGrant_GOV_003(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	member, _ := g.ids.New()
	if err := g.db.InTx(ctx, storage.Tenant{OrganizationID: g.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO memberships (id, organization_id, user_id, team_id, role) VALUES ($1, $2, $3, $4, 'member')`, member, g.org, g.bobID, g.team)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.svc.CreateGrant(ctx, g.owner, auth.GrantSpec{TeamID: g.team, Role: "auditor", Scope: auth.ScopeOrganization}); err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}
	bob := g.principalOf(t, "bob@example.com", g.org)
	if !bob.Holds(auth.AuditRead) || !bob.Holds(auth.AuditExport) || !bob.Holds(auth.ReconstructionView) {
		t.Errorf("the team's auditor role did not reach its member: %v", bob.Permissions)
	}
	if bob.Holds(auth.PluginEdit) {
		t.Error("the auditor role let a member edit")
	}
	if err := g.db.InTx(ctx, storage.Tenant{OrganizationID: g.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM memberships WHERE id = $1`, member)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if again := g.principalOf(t, "bob@example.com", g.org); again.Holds(auth.AuditRead) {
		t.Error("a former member still holds the team's role")
	}
}

// Verifies: GOV-002.
// The last owner cannot be removed, directly or by a grant (PLX-8103).
func TestLastOwner_GOV_002(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	ctx := context.Background()
	// Bob becomes an owner by a grant; Ada is one by membership.
	grant, err := g.svc.CreateGrant(ctx, g.owner, auth.GrantSpec{UserID: g.bobID, Role: "owner", Scope: auth.ScopeOrganization})
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}
	if err := g.svc.DeleteGrant(ctx, g.owner, grant.ID); err != nil {
		t.Errorf("revoking an owner while another remains: %v", err)
	}
	grant, err = g.svc.CreateGrant(ctx, g.owner, auth.GrantSpec{UserID: g.bobID, Role: "owner", Scope: auth.ScopeOrganization})
	if err != nil {
		t.Fatal(err)
	}
	// Ada's membership is removed behind the service's back; Bob is now the
	// only owner, by grant.
	if err := g.db.InTx(ctx, storage.Tenant{OrganizationID: g.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM memberships WHERE organization_id = $1 AND team_id IS NULL AND role = 'owner'`, g.org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.svc.UpdateGrant(ctx, g.owner, grant.ID, "admin"); code(err) != plxerr.LastOwnerCannotBeRemoved {
		t.Errorf("demoting the last owner: %v", err)
	}
	if err := g.svc.DeleteGrant(ctx, g.owner, grant.ID); code(err) != plxerr.LastOwnerCannotBeRemoved {
		t.Errorf("revoking the last owner: %v", err)
	}
	// The refusals rolled back: the grant is still there.
	got, err := g.svc.ListGrants(ctx, g.owner, auth.GrantFilter{}, storage.Cursor{}, 10)
	if err != nil || len(got) != 1 || got[0].Role != "owner" {
		t.Errorf("grants after the refusals = %+v, %v", got, err)
	}
}

// Verifies: GOV-002.
// Custom roles that exist keep resolving when the feature is off, so a
// lapsed licence never changes anyone's access.
func TestCustomRolesResolveWhenTheFeatureIsOff_GOV_002(t *testing.T) {
	t.Parallel()
	enabled := true
	g := newGovernance(t, func(o *auth.Options) { o.CustomRolesEnabled = func() bool { return enabled } })
	ctx := context.Background()
	if _, err := g.svc.CreateRole(ctx, g.owner, "log-reader", "Log reader", "", []string{"audit.read"}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.svc.CreateGrant(ctx, g.owner, auth.GrantSpec{UserID: g.bobID, Role: "log-reader", Scope: auth.ScopeOrganization}); err != nil {
		t.Fatal(err)
	}
	enabled = false
	if bob := g.principalOf(t, "bob@example.com", g.org); !bob.Holds(auth.AuditRead) {
		t.Error("a custom role stopped resolving when the feature went off")
	}
	if _, err := g.svc.UpdateRole(ctx, g.owner, "log-reader", "Log reader", "", []string{"audit.read", "app.read"}); code(err) != plxerr.EnterpriseFeatureNotLicensed {
		t.Errorf("changing a custom role with the feature off: %v", err)
	}
}

// licence is a provider with a fixed status.
type licence edition.Status

func (l licence) Status() edition.Status { return edition.Status(l) }

// Verifies: GOV-002, GOV-033.
// Without an override, the licence in the request's context decides whether
// custom roles may be created: no provider is the community edition, a
// valid licence listing the feature allows it, an expired one does not.
func TestCustomRolesFollowTheLicenceInTheContext_GOV_033(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	features := []edition.Feature{edition.FeatureCustomRoles}
	for _, c := range []struct {
		name, key string
		ctx       context.Context
		want      plxerr.Code
	}{
		{"community", "reader-a", context.Background(), plxerr.EnterpriseFeatureNotLicensed},
		{"valid without the feature", "reader-b", edition.WithProvider(context.Background(), licence{State: edition.StateValid}), plxerr.EnterpriseFeatureNotLicensed},
		{"expired", "reader-c", edition.WithProvider(context.Background(), licence{State: edition.StateExpired, Features: features}), plxerr.EnterpriseFeatureNotLicensed},
		{"grace", "reader-d", edition.WithProvider(context.Background(), licence{State: edition.StateGrace, Features: features}), 0},
		{"valid", "reader-e", edition.WithProvider(context.Background(), licence{State: edition.StateValid, Features: features}), 0},
	} {
		_, err := g.svc.CreateRole(c.ctx, g.owner, c.key, "Reader", "", []string{"audit.read"})
		if c.want == 0 {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			continue
		}
		if code(err) != c.want {
			t.Errorf("%s: %v; want %v", c.name, err, c.want)
		}
	}
}
