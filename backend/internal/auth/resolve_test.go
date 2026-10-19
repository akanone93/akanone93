// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"slices"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

func errCode(err error) plxerr.Code {
	if e, ok := err.(*plxerr.Error); ok { //nolint:errorlint // the principal returns the error itself
		return e.Code
	}
	return 0
}

func session(second bool) Identity {
	return Identity{Kind: KindUser, ID: "u", UserID: "u", SecondFactor: second}
}

// Verifies: GOV-001, GOV-003.
// Grants on the organisation, an app and an environment resolve to the
// bucket each belongs in, and an app grant carries what the app level
// always carried: every permission that is not the organisation's.
func TestResolveScopes_GOV_003(t *testing.T) {
	t.Parallel()
	p := resolve(session(true), "org", []roleGrant{
		{Role: RoleViewer, Scope: ScopeOrganization},
		{Role: RoleDeveloper, Scope: ScopeApp, AppID: "shop"},
		{Role: RoleReleaseManager, Scope: ScopeEnvironment, AppID: "bank", Environment: "production"},
	}, nil, nil)
	if !p.Holds(AppRead) || p.Holds(PluginEdit) {
		t.Errorf("organisation permissions = %v", p.Permissions)
	}
	if !p.HoldsOnApp(PluginEdit, "shop") || p.HoldsOnApp(PluginEdit, "bank") {
		t.Errorf("app permissions = %v", p.AppPermissions)
	}
	if !p.HoldsOnEnvironment(ReleasePublish, "bank", "production") ||
		p.HoldsOnEnvironment(ReleasePublish, "bank", "development") ||
		p.HoldsOnEnvironment(ReleasePublish, "other", "production") {
		t.Errorf("environment permissions = %v", p.EnvPermissions)
	}
	// An app grant of the developer role reaches every environment of the app.
	if !p.HoldsOnEnvironment(ReleasePublish, "shop", "anything") {
		t.Error("an app grant did not reach the app's environments")
	}
	// An environment grant carries environment permissions only.
	for scope, perms := range p.EnvPermissions {
		for _, perm := range perms {
			if info, _ := Describe(perm); info.Scope != ScopeEnvironment {
				t.Errorf("%v holds %s, which cannot be limited to an environment", scope, perm)
			}
		}
	}
}

// Verifies: GOV-001.
// The pre-P9 permissions an app grant carries are exactly the pre-P9
// app-level set for the roles that existed before P9; the owner's grant
// also carries the P9 permissions that can be held on an app.
func TestAppGrantCarriesTheSameAsBefore_GOV_001(t *testing.T) {
	t.Parallel()
	appLevel := []Permission{
		A11yWaive, AppManage, AppRead, EnvironmentManage, FunctionDeploy, FunctionDeployProduction,
		FunctionRead, GoldensApprove, PluginEdit, PluginLockOverride, PluginRead,
		ReleasePromote, ReleasePublish, SecretsManage, TrashManage,
		TranslationsApprove, TranslationsEdit,
	}
	isP9 := func(perm Permission) bool {
		return !slices.Contains([]Permission{
			A11yWaive, AppManage, AppRead, AuditRead, EnvironmentManage, FunctionDeploy, FunctionDeployProduction,
			FunctionRead, GoldensApprove, KeysManage, LimitsManage, MembersManage, OrganizationManage,
			OrganizationRead, PluginEdit, PluginLockOverride, PluginRead, ReleasePromote, ReleasePublish,
			SecretsManage, TokensManage, TranslationsApprove, TranslationsEdit, TrashManage,
		}, perm)
	}
	for _, role := range []Role{RoleOwner, RoleAdmin, RoleDeveloper, RoleViewer, RoleTranslator} {
		var want []Permission
		for _, perm := range RolePermissions(role) {
			if slices.Contains(appLevel, perm) {
				want = append(want, perm)
			}
		}
		slices.Sort(want)
		p := resolve(session(true), "org", []roleGrant{{Role: role, Scope: ScopeApp, AppID: "shop"}}, nil, nil)
		got := slices.DeleteFunc(slices.Clone(p.AppPermissions["shop"]), isP9)
		if !slices.Equal(got, want) {
			t.Errorf("%s on an app: %v, want %v", role, got, want)
		}
		if len(p.Permissions) != 0 || len(p.EnvPermissions) != 0 {
			t.Errorf("%s on an app also holds %v %v", role, p.Permissions, p.EnvPermissions)
		}
	}
}

// Verifies: GOV-003.
// A permission written <permission>:<environment> applies to that
// environment within the grant's scope, and a mismatching environment grant
// drops it.
func TestResolveScopedPermissions_GOV_003(t *testing.T) {
	t.Parallel()
	custom := map[Role][]ScopedPermission{
		"prod-publisher": {{Permission: ReleasePublish, Environment: "production"}, {Permission: AppRead}},
	}
	org := resolve(session(true), "org", []roleGrant{{Role: "prod-publisher", Scope: ScopeOrganization}}, custom, nil)
	if !org.Holds(AppRead) || org.Holds(ReleasePublish) {
		t.Errorf("organisation = %v", org.Permissions)
	}
	if !org.HoldsOnEnvironment(ReleasePublish, "any-app", "production") || org.HoldsOnEnvironment(ReleasePublish, "any-app", "staging") {
		t.Errorf("organisation environment permissions = %v", org.EnvPermissions)
	}
	app := resolve(session(true), "org", []roleGrant{{Role: "prod-publisher", Scope: ScopeApp, AppID: "shop"}}, custom, nil)
	if !app.HoldsOnEnvironment(ReleasePublish, "shop", "production") || app.HoldsOnEnvironment(ReleasePublish, "bank", "production") {
		t.Errorf("app environment permissions = %v", app.EnvPermissions)
	}
	match := resolve(session(true), "org", []roleGrant{{Role: "prod-publisher", Scope: ScopeEnvironment, AppID: "shop", Environment: "production"}}, custom, nil)
	if !match.HoldsOnEnvironment(ReleasePublish, "shop", "production") {
		t.Errorf("a matching environment grant = %v", match.EnvPermissions)
	}
	mismatch := resolve(session(true), "org", []roleGrant{{Role: "prod-publisher", Scope: ScopeEnvironment, AppID: "shop", Environment: "staging"}}, custom, nil)
	if mismatch.HoldsOnEnvironment(ReleasePublish, "shop", "production") || mismatch.HoldsOnEnvironment(ReleasePublish, "shop", "staging") {
		t.Errorf("a mismatching environment grant = %v", mismatch.EnvPermissions)
	}
}

// Verifies: SEC-102, GOV-002.
// A role that is not known holds nothing, and a permission the catalogue
// no longer has is not held.
func TestResolveUnknownRolesHoldNothing_GOV_002(t *testing.T) {
	t.Parallel()
	custom := map[Role][]ScopedPermission{"old": {{Permission: "retired.permission"}}}
	p := resolve(session(true), "org", []roleGrant{
		{Role: "wizard", Scope: ScopeOrganization},
		{Role: "old", Scope: ScopeOrganization},
		{Role: "deleted-role", Scope: ScopeApp, AppID: "shop"},
	}, custom, nil)
	if len(p.Permissions) != 0 || len(p.AppPermissions) != 0 || len(p.EnvPermissions) != 0 {
		t.Errorf("principal = %+v", p)
	}
}

// Verifies: SEC-102, GOV-003.
// A token never holds more than its scopes, at any level.
func TestResolveTokenScopes_GOV_003(t *testing.T) {
	t.Parallel()
	id := Identity{Kind: KindToken, ID: "t", UserID: "u", Scopes: []Permission{ReleasePublish}, SecondFactor: true}
	p := resolve(id, "org", []roleGrant{
		{Role: RoleAdmin, Scope: ScopeApp, AppID: "shop"},
		{Role: RoleReleaseManager, Scope: ScopeEnvironment, AppID: "bank", Environment: "production"},
	}, nil, nil)
	if got := p.AppPermissions["shop"]; !slices.Equal(got, []Permission{ReleasePublish}) {
		t.Errorf("app permissions = %v", got)
	}
	if got := p.EnvPermissions[EnvScope{AppID: "bank", Environment: "production"}]; !slices.Equal(got, []Permission{ReleasePublish}) {
		t.Errorf("environment permissions = %v", got)
	}
	none := resolve(Identity{Kind: KindToken, Scopes: []Permission{}}, "org", []roleGrant{{Role: RoleOwner, Scope: ScopeOrganization}}, nil, nil)
	if len(none.Permissions) != 0 {
		t.Errorf("a token with no scopes holds %v", none.Permissions)
	}
}

// Verifies: SEC-102, SEC-100, GOV-003.
// AuthorizeEnvironment is deny by default, refuses a missing second
// factor for what it is, and AuthorizeSomeEnvironment passes only for a
// principal who may act somewhere on the app.
func TestAuthorizeEnvironment_GOV_003(t *testing.T) {
	t.Parallel()
	grants := []roleGrant{{Role: RoleReleaseManager, Scope: ScopeEnvironment, AppID: "shop", Environment: "development"}}
	p := resolve(session(true), "org", grants, nil, nil)
	if err := p.AuthorizeEnvironment(ReleasePublish, "shop", "development"); err != nil {
		t.Errorf("development: %v", err)
	}
	if err := p.AuthorizeEnvironment(ReleasePublish, "shop", "production"); errCode(err) != plxerr.PermissionDenied {
		t.Errorf("production: %v", err)
	}
	if err := p.AuthorizeSomeEnvironment(ReleasePublish, "shop"); err != nil {
		t.Errorf("somewhere on the app: %v", err)
	}
	if err := p.AuthorizeSomeEnvironment(ReleasePublish, "bank"); errCode(err) != plxerr.PermissionDenied {
		t.Errorf("somewhere on another app: %v", err)
	}
	if err := p.AuthorizeSomeEnvironment(AppManage, "shop"); errCode(err) != plxerr.PermissionDenied {
		t.Errorf("a permission the grant does not carry: %v", err)
	}
	without := resolve(session(false), "org", grants, nil, nil)
	if err := without.AuthorizeEnvironment(ReleasePublish, "shop", "development"); errCode(err) != plxerr.MultiFactorRequired {
		t.Errorf("without a second factor: %v", err)
	}
	var nobody Principal
	if err := nobody.AuthorizeEnvironment(AppRead, "shop", "development"); errCode(err) != plxerr.AuthenticationRequired {
		t.Errorf("without an organisation: %v", err)
	}
	// An organisation-wide holder holds on every environment.
	org := resolve(session(true), "org", []roleGrant{{Role: RoleAdmin, Scope: ScopeOrganization}}, nil, nil)
	if err := org.AuthorizeEnvironment(ReleasePublish, "any", "any"); err != nil {
		t.Errorf("an administrator: %v", err)
	}
}
