// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"fmt"
	"slices"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: GOV-002, GOV-006.
// A pattern names a catalogue entry, "*", an area or a feature, and nothing
// else; the wildcards follow the catalogue's area and feature fields.
func TestMatchesPattern_GOV_006(t *testing.T) {
	t.Parallel()
	publish, _ := Describe(ReleasePublish)
	lock, _ := Describe(PluginLockOverride)
	for _, c := range []struct {
		pattern string
		info    PermissionInfo
		want    bool
	}{
		{"*", publish, true},
		{"release.publish", publish, true},
		{"release.*", publish, true},
		{"release.release.*", publish, true},
		{"release.promote", publish, false},
		{"plugin.*", publish, false},
		{"plugin.*", lock, true},
		{"plugin.plugin.*", lock, true},
		{"plugin.lock.*", lock, false},
		{"release", publish, false},
		{"*.publish", publish, false},
		{"", publish, false},
	} {
		if got := matchesPattern(c.pattern, c.info); got != c.want {
			t.Errorf("matchesPattern(%q, %s) = %v, want %v", c.pattern, c.info.Name, got, c.want)
		}
	}
}

// Verifies: GOV-006.
// Patterns are validated against the catalogue, de-duplicated and sorted;
// one that names nothing is PLX-8104.
func TestCheckPatterns_GOV_006(t *testing.T) {
	t.Parallel()
	got, err := checkPatterns([]string{" plugin.* ", "app.read", "plugin.*"})
	if err != nil || !slices.Equal(got, []string{"app.read", "plugin.*"}) {
		t.Errorf("checkPatterns = %v, %v", got, err)
	}
	for _, bad := range [][]string{nil, {}, {"nothing.*"}, {"plugin.edit", "plugin.nonsense"}, {""}, {"*.*"}, {"plugin."}} {
		if _, err := checkPatterns(bad); errCode(err) != plxerr.RestrictionPatternInvalid {
			t.Errorf("checkPatterns(%q) = %v, want PLX-8104", bad, err)
		}
	}
}

// grantScope is where a test principal holds the developer role.
type grantScope struct {
	name  string
	grant roleGrant
}

// scopeOfRestriction is where a test restriction applies.
type scopeOfRestriction struct {
	name               string
	appID, environment string
}

// decisions asks every question the resolver answers, naming each.
func decisions(p Principal, perm Permission) map[string]bool {
	return map[string]bool{
		"organisation":  p.Holds(perm),
		"app A":         p.HoldsOnApp(perm, "A"),
		"app B":         p.HoldsOnApp(perm, "B"),
		"A production":  p.HoldsOnEnvironment(perm, "A", "production"),
		"A development": p.HoldsOnEnvironment(perm, "A", "development"),
		"B production":  p.HoldsOnEnvironment(perm, "B", "production"),
		"A somewhere":   p.HoldsInSomeEnvironment(perm, "A"),
	}
}

// expected is the decision the model gives: a grant covers some requests, a
// restriction covers some, and a request is allowed when the grant covers
// it and the restriction does not.
func expected(grant grantScope, rs scopeOfRestriction, question string) bool {
	g := grant.grant
	var app, env string
	switch question {
	case "organisation":
	case "app A", "app B":
		app = question[len(question)-1:]
	case "A production":
		app, env = "A", "production"
	case "A development":
		app, env = "A", "development"
	case "B production":
		app, env = "B", "production"
	case "A somewhere":
		return expected(grant, rs, "app A") || expected(grant, rs, "A production") || expected(grant, rs, "A development")
	}
	covered := false
	switch g.Scope {
	case ScopeOrganization:
		covered = true
	case ScopeApp:
		covered = app == g.AppID
	case ScopeEnvironment:
		covered = app == g.AppID && env == g.Environment
	}
	denied := false
	switch {
	case rs.appID == "":
		denied = true
	case rs.environment == "":
		denied = app == rs.appID
	default:
		denied = app == rs.appID && env == rs.environment
	}
	return covered && !denied
}

// Verifies: GOV-006, SEC-102.
// Deny wins: whatever grants a permission, a restriction that names it, by
// name or by any wildcard, removes it exactly where the restriction applies
// and nowhere else; a permission it does not name is untouched. The table
// crosses grants on the organisation, an app and an environment with
// restrictions on the organisation, an app and an environment, and four
// ways of naming the permission.
func TestDenyWinsOverEveryGrant_GOV_006(t *testing.T) {
	t.Parallel()
	grantsAt := []grantScope{
		{"organisation grant", roleGrant{Role: RoleDeveloper, Scope: ScopeOrganization}},
		{"app A grant", roleGrant{Role: RoleDeveloper, Scope: ScopeApp, AppID: "A"}},
		{"A production grant", roleGrant{Role: RoleDeveloper, Scope: ScopeEnvironment, AppID: "A", Environment: "production"}},
	}
	restrictionsAt := []scopeOfRestriction{
		{"on the organisation", "", ""},
		{"on app A", "A", ""},
		{"on A production", "A", "production"},
	}
	patterns := []string{"release.publish", "release.*", "release.release.*", "*"}
	for _, g := range grantsAt {
		for _, rs := range restrictionsAt {
			for _, pattern := range patterns {
				t.Run(fmt.Sprintf("%s/%s/%s", g.name, rs.name, pattern), func(t *testing.T) {
					t.Parallel()
					r := Restriction{
						ID: "r", SubjectKind: SubjectUser, SubjectID: "u", Patterns: []string{pattern},
						AppID: rs.appID, Environment: rs.environment,
					}
					p := resolve(session(true), "org", []roleGrant{g.grant}, nil, nil, r)
					for question, got := range decisions(p, ReleasePublish) {
						if want := expected(g, rs, question); got != want {
							t.Errorf("%s: holds release.publish = %v, want %v", question, got, want)
						}
					}
				})
			}
		}
	}
}

// Verifies: GOV-006.
// A restriction that names other permissions leaves the permission alone,
// and the principal records nothing as removed.
func TestRestrictionThatNamesOtherPermissions_GOV_006(t *testing.T) {
	t.Parallel()
	r := Restriction{ID: "r", SubjectKind: SubjectOrganization, Patterns: []string{"release.promote", "plugin.*"}}
	p := resolve(session(true), "org", []roleGrant{{Role: RoleDeveloper, Scope: ScopeOrganization}}, nil, nil, r)
	if !p.Holds(ReleasePublish) || p.Holds(PluginEdit) || p.Holds(PluginRead) {
		t.Errorf("permissions = %v", p.Permissions)
	}
	if len(p.Restrictions) != 1 || !slices.Equal(p.Restrictions[0].Removed, []Permission{PluginEdit, PluginRead}) {
		t.Errorf("applied = %+v", p.Restrictions)
	}
}

// Verifies: GOV-006, SEC-102.
// The order of the restrictions and of the grants does not matter: every
// ordering gives the same permissions.
func TestRestrictionOrderDoesNotMatter_GOV_006(t *testing.T) {
	t.Parallel()
	given := []roleGrant{
		{Role: RoleViewer, Scope: ScopeOrganization},
		{Role: RoleDeveloper, Scope: ScopeApp, AppID: "A"},
		{Role: RoleReleaseManager, Scope: ScopeEnvironment, AppID: "A", Environment: "production"},
	}
	rs := []Restriction{
		{ID: "1", SubjectKind: SubjectOrganization, Patterns: []string{"plugin.read"}},
		{ID: "2", SubjectKind: SubjectTeam, SubjectID: "t", Patterns: []string{"release.*"}, AppID: "A"},
		{ID: "3", SubjectKind: SubjectUser, SubjectID: "u", Patterns: []string{"app.*"}, AppID: "A", Environment: "production"},
	}
	want := decisionsOf(resolve(session(true), "org", given, nil, nil, rs...))
	for _, order := range [][]int{{0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		reordered := []Restriction{rs[order[0]], rs[order[1]], rs[order[2]]}
		for _, grants := range [][]roleGrant{given, {given[2], given[0], given[1]}, {given[1], given[2], given[0]}} {
			if got := decisionsOf(resolve(session(true), "org", grants, nil, nil, reordered...)); !slices.Equal(got, want) {
				t.Fatalf("order %v: decisions %v, want %v", order, got, want)
			}
		}
	}
}

// decisionsOf flattens the decisions over a few permissions into a sorted
// list.
func decisionsOf(p Principal) []string {
	var out []string
	for _, perm := range []Permission{AppRead, PluginRead, PluginEdit, ReleasePublish, ReleasePromote} {
		for question, held := range decisions(p, perm) {
			out = append(out, fmt.Sprintf("%s %s %v", perm, question, held))
		}
	}
	slices.Sort(out)
	return out
}

// Verifies: GOV-006.
// An organisation permission restricted on app A fails on A and still
// passes on app B, as a permission that is not one of the app's is never
// taken by a restriction limited to an app.
func TestOrganisationPermissionRestrictedOnOneApp_GOV_006(t *testing.T) {
	t.Parallel()
	r := Restriction{ID: "r", SubjectKind: SubjectUser, SubjectID: "u", Patterns: []string{"*"}, AppID: "A"}
	p := resolve(session(true), "org", []roleGrant{{Role: RoleAdmin, Scope: ScopeOrganization}}, nil, nil, r)
	if err := p.AuthorizeApp(PluginEdit, "A"); errCode(err) != plxerr.PermissionDenied {
		t.Errorf("AuthorizeApp(plugin.edit, A) = %v, want denied", err)
	}
	if err := p.AuthorizeApp(PluginEdit, "B"); err != nil {
		t.Errorf("AuthorizeApp(plugin.edit, B) = %v", err)
	}
	if err := p.Authorize(OrganizationManage); err != nil {
		t.Errorf("Authorize(organization.manage) = %v; an app restriction does not touch organisation permissions", err)
	}
	if err := p.AuthorizeEnvironment(ReleasePublish, "A", "production"); errCode(err) != plxerr.PermissionDenied {
		t.Errorf("AuthorizeEnvironment on A = %v, want denied", err)
	}
	if err := p.AuthorizeSomeEnvironment(ReleasePublish, "A"); errCode(err) != plxerr.PermissionDenied {
		t.Errorf("AuthorizeSomeEnvironment on A = %v, want denied", err)
	}
}

// Verifies: GOV-006.
// A restriction on one environment of an app cuts a grant on that
// environment key across every app only for the restricted app.
func TestRestrictionOnOneEnvironment_GOV_006(t *testing.T) {
	t.Parallel()
	r := Restriction{ID: "r", SubjectKind: SubjectUser, SubjectID: "u", Patterns: []string{"release.publish"}, AppID: "A", Environment: "production"}
	custom := map[Role][]ScopedPermission{"prod-publisher": {{Permission: ReleasePublish, Environment: "production"}}}
	p := resolve(session(true), "org", []roleGrant{{Role: "prod-publisher", Scope: ScopeOrganization}}, custom, nil, r)
	if p.HoldsOnEnvironment(ReleasePublish, "A", "production") {
		t.Error("the restricted environment of A still publishes")
	}
	if !p.HoldsOnEnvironment(ReleasePublish, "B", "production") {
		t.Error("the same environment of app B lost the permission")
	}
	if p.HoldsInSomeEnvironment(ReleasePublish, "A") {
		t.Error("app A still publishes somewhere")
	}
}

// Verifies: GOV-006, SEC-102.
// An Owner keeps the owner floor whatever restricts them, loses the rest, and
// the floor is not recorded as removed.
func TestOwnerFloor_GOV_006(t *testing.T) {
	t.Parallel()
	all := Restriction{ID: "r", SubjectKind: SubjectOrganization, Patterns: []string{"*"}}
	owner := resolve(session(true), "org", []roleGrant{{Role: RoleOwner, Scope: ScopeOrganization}}, nil, nil, all)
	if want := slices.Sorted(slices.Values(ownerFloor)); !slices.Equal(owner.Permissions, want) {
		t.Errorf("an owner under * holds %v, want the floor %v", owner.Permissions, want)
	}
	for _, perm := range ownerFloor {
		if err := owner.Authorize(perm); err != nil {
			t.Errorf("owner Authorize(%s) = %v", perm, err)
		}
	}
	if err := owner.Authorize(PluginRead); errCode(err) != plxerr.PermissionDenied {
		t.Errorf("owner Authorize(plugin.read) = %v, want denied", err)
	}
	for _, perm := range owner.Restrictions[0].Removed {
		if slices.Contains(ownerFloor, perm) {
			t.Errorf("%s is recorded as removed but the floor gave it back", perm)
		}
	}
	admin := resolve(session(true), "org", []roleGrant{{Role: RoleAdmin, Scope: ScopeOrganization}}, nil, nil, all)
	if len(admin.Permissions) != 0 {
		t.Errorf("an admin under * holds %v", admin.Permissions)
	}
	appOwner := resolve(session(true), "org", []roleGrant{{Role: RoleOwner, Scope: ScopeApp, AppID: "A"}}, nil, nil, all)
	if len(appOwner.Permissions) != 0 || len(appOwner.AppPermissions) != 0 {
		t.Errorf("an owner of one app got the floor: %v %v", appOwner.Permissions, appOwner.AppPermissions)
	}
}

// Verifies: GOV-006, SEC-102.
// The owner floor is also bounded by a token's scopes: a token never holds
// what its creator's session could not have given it.
func TestOwnerFloorAndTokenScopes_GOV_006(t *testing.T) {
	t.Parallel()
	all := Restriction{ID: "r", SubjectKind: SubjectOrganization, Patterns: []string{"*"}}
	token := session(true)
	token.Kind, token.Scopes = KindToken, []Permission{MembersManage, PluginEdit}
	p := resolve(token, "org", []roleGrant{{Role: RoleOwner, Scope: ScopeOrganization}}, nil, nil, all)
	if !slices.Equal(p.Permissions, []Permission{MembersManage}) {
		t.Errorf("token permissions = %v, want [members.manage]", p.Permissions)
	}
}

// Verifies: GOV-006, SEC-102.
// A restriction on the organisation also takes permissions from a workload
// identity, whose permissions are its token's scopes.
func TestRestrictionBindsAWorkload_GOV_006(t *testing.T) {
	t.Parallel()
	ci := Identity{Kind: KindCI, ID: "t", Scopes: []Permission{PluginRead, ReleasePublish}}
	r := Restriction{ID: "r", SubjectKind: SubjectOrganization, Patterns: []string{"release.*"}}
	p := resolve(ci, "org", nil, nil, ci.Scopes, r)
	if !slices.Equal(p.Permissions, []Permission{PluginRead}) {
		t.Errorf("workload permissions = %v", p.Permissions)
	}
}

// Verifies: GOV-006.
// Without restrictions the principal carries none, and resolving as before
// gives the same permissions.
func TestNoRestrictionsChangeNothing_GOV_006(t *testing.T) {
	t.Parallel()
	p := resolve(session(true), "org", []roleGrant{{Role: RoleDeveloper, Scope: ScopeOrganization}}, nil, nil)
	if p.Restrictions != nil || !p.Holds(PluginEdit) {
		t.Errorf("principal = %+v", p)
	}
}

// Verifies: GOV-006.
// A restriction names only what the catalogue has, so the owner floor is
// four permissions of the catalogue that need no app.
func TestOwnerFloorIsOrganisationPermissions_GOV_006(t *testing.T) {
	t.Parallel()
	for _, perm := range OwnerFloor() {
		info, ok := Describe(perm)
		if !ok || info.Scope != ScopeOrganization {
			t.Errorf("%s in the floor: %+v", perm, info)
		}
	}
	floor := OwnerFloor()
	floor[0] = "changed"
	if OwnerFloor()[0] == "changed" {
		t.Error("OwnerFloor returned the table itself")
	}
}
