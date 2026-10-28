// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// permissionConstants reads the Permission constants the package declares in
// rbac.go and rbac_p9.go.
func permissionConstants(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, file := range []string{"rbac.go", "rbac_p9.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "Permission" {
					continue
				}
				lit, ok := vs.Values[0].(*ast.BasicLit)
				if !ok {
					t.Fatalf("%s: %s is not a string literal", file, vs.Names[0].Name)
				}
				name, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				out = append(out, name)
			}
		}
	}
	slices.Sort(out)
	return out
}

// Verifies: GOV-002.
// The Permission constants and the catalogue are the same set: a constant
// added without a catalogue entry, or an entry without a constant, fails
// here.
func TestCatalogueEqualsTheConstants_GOV_002(t *testing.T) {
	t.Parallel()
	var names []string
	for _, e := range auth.Catalogue() {
		names = append(names, string(e.Name))
	}
	if got := permissionConstants(t); !slices.Equal(got, names) {
		t.Errorf("constants and catalogue differ:\nconstants: %v\ncatalogue: %v", got, names)
	}
}

// Verifies: GOV-002.
// Every permission has a description, a risk, a scope and its area, feature
// and action, which spell its name; the P9 permissions of the plan are all
// in the catalogue.
func TestCatalogueEntries_GOV_002(t *testing.T) {
	t.Parallel()
	cat := auth.Catalogue()
	if len(cat) != 55 {
		t.Errorf("the catalogue has %d permissions, want 55 (24 existing, 26 of P9, 2 of P10, 3 of P11)", len(cat))
	}
	for i, e := range cat {
		if e.Description == "" || !strings.HasSuffix(e.Description, ".") {
			t.Errorf("%s: description %q", e.Name, e.Description)
		}
		if !slices.Contains([]auth.Risk{auth.RiskLow, auth.RiskMedium, auth.RiskHigh}, e.Risk) {
			t.Errorf("%s: risk %q", e.Name, e.Risk)
		}
		if !slices.Contains([]auth.Scope{auth.ScopeOrganization, auth.ScopeApp, auth.ScopeEnvironment}, e.Scope) {
			t.Errorf("%s: scope %q", e.Name, e.Scope)
		}
		if i > 0 && cat[i-1].Name >= e.Name {
			t.Errorf("%s is out of order", e.Name)
		}
		if e.Area == "" || e.Feature == "" || e.Action == "" {
			t.Errorf("%s: area %q, feature %q, action %q; each is required", e.Name, e.Area, e.Feature, e.Action)
		}
		if name := string(e.Name); name != e.Area+"."+e.Action && name != e.Area+"."+e.Feature+"."+e.Action {
			t.Errorf("%s: area %q, feature %q and action %q do not spell the name", e.Name, e.Area, e.Feature, e.Action)
		}
		if e.ReadOnly && e.Risk == auth.RiskHigh && !e.StepUp {
			t.Errorf("%s: a read-only permission of high risk needs a second factor", e.Name)
		}
		if got, ok := auth.Describe(e.Name); !ok || got != e {
			t.Errorf("Describe(%s) = %+v, %v", e.Name, got, ok)
		}
	}
	p9 := []auth.Permission{
		auth.RolesManage, auth.TeamsManage, auth.SSOManage, auth.SessionsManage, auth.PoliciesManage,
		auth.ApprovalsDecide, auth.BreakglassInvoke, auth.FreezesManage, auth.ComplianceReport,
		auth.RolloutManage, auth.KillswitchManage, auth.FlagsManage, auth.ExperimentsManage,
		auth.AnalyticsRead, auth.AlertsManage, auth.NotificationsManage, auth.WebhooksManage,
		auth.AuditExport, auth.ReconstructionView, auth.DSRManage, auth.SearchRead, auth.GitExport,
		auth.LicenseManage, auth.BackupManage, auth.RetentionManage, auth.RestrictionsManage,
	}
	for _, p := range p9 {
		if !auth.InCatalogue(p) {
			t.Errorf("%s is not in the catalogue", p)
		}
	}
	if _, ok := auth.Describe("ghosts.manage"); ok {
		t.Error("an unknown permission was described")
	}
}

// Verifies: GOV-002, SEC-100.
// The second-factor rule of the permissions that existed before P9 is
// unchanged by the catalogue.
func TestStepUpOfExistingPermissionsIsUnchanged_GOV_002(t *testing.T) {
	t.Parallel()
	was := []auth.Permission{
		auth.A11yWaive, auth.FunctionDeployProduction, auth.GoldensApprove, auth.KeysManage, auth.MembersManage,
		auth.ReleasePromote, auth.ReleasePublish, auth.SecretsManage, auth.TokensManage, auth.TranslationsApprove,
	}
	pre := []auth.Permission{
		auth.A11yWaive, auth.AppManage, auth.AppRead, auth.AuditRead, auth.EnvironmentManage, auth.FunctionDeploy,
		auth.FunctionDeployProduction, auth.FunctionRead, auth.GoldensApprove, auth.KeysManage, auth.LimitsManage,
		auth.MembersManage, auth.OrganizationManage, auth.OrganizationRead, auth.PluginEdit,
		auth.PluginLockOverride, auth.PluginRead, auth.ReleasePromote, auth.ReleasePublish,
		auth.SecretsManage, auth.TokensManage, auth.TranslationsApprove, auth.TranslationsEdit, auth.TrashManage,
	}
	for _, p := range pre {
		if got, want := auth.NeedsSecondFactor(p), slices.Contains(was, p); got != want {
			t.Errorf("%s needs a second factor = %v, was %v", p, got, want)
		}
	}
}

// Verifies: GOV-002, SEC-102.
// Nobody gains or loses a permission of the five roles that existed before
// P9: each holds exactly what it held, and the owner also holds the P9
// permissions, which guard only calls that did not exist. The one addition
// is restrictions.manage, which Admins hold beside the Owner (GOV-006).
// P10 adds the two dev-session permissions: the Admin and the Developer hold
// both, and the Viewer may observe (DEV-020, ADR-0087).
// P11 adds the two library permissions, which the Admin holds (ADR-0098),
// and analytics.manage, which the Admin holds (ANL-020).
func TestExistingRolesKeepTheirGrants_GOV_002(t *testing.T) {
	t.Parallel()
	held := map[auth.Role][]auth.Permission{
		auth.RoleAdmin: {
			auth.AppManage, auth.AppRead, auth.AuditRead, auth.EnvironmentManage, auth.FunctionDeploy,
			auth.FunctionDeployProduction, auth.FunctionRead, auth.LimitsManage,
			auth.OrganizationManage, auth.OrganizationRead, auth.PluginEdit, auth.PluginLockOverride,
			auth.PluginRead, auth.ReleasePromote, auth.ReleasePublish, auth.SecretsManage, auth.TokensManage,
			auth.TrashManage, auth.A11yWaive, auth.GoldensApprove, auth.TranslationsApprove, auth.TranslationsEdit,
			auth.RestrictionsManage, auth.DevSessionCreate, auth.DevSessionObserve,
			auth.LibraryManage, auth.LibraryPublish, auth.AnalyticsManage,
		},
		auth.RoleDeveloper: {
			auth.AppRead, auth.FunctionDeploy, auth.FunctionRead, auth.OrganizationRead, auth.PluginEdit,
			auth.PluginRead, auth.ReleasePublish, auth.TranslationsEdit, auth.DevSessionCreate, auth.DevSessionObserve,
		},
		auth.RoleViewer:     {auth.AppRead, auth.DevSessionObserve, auth.FunctionRead, auth.OrganizationRead, auth.PluginRead},
		auth.RoleTranslator: {auth.AppRead, auth.OrganizationRead, auth.PluginRead, auth.TranslationsEdit},
	}
	for role, want := range held {
		got := auth.RolePermissions(role)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s holds %v, held %v", role, got, want)
		}
	}
	// The owner held the whole catalogue and holds the whole catalogue.
	if got := auth.RolePermissions(auth.RoleOwner); !slices.Equal(got, auth.Permissions()) {
		t.Errorf("the owner holds %d of %d permissions", len(got), len(auth.Permissions()))
	}
}

// Verifies: GOV-002.
// The ten predefined roles exist, in the order of the specification; the
// auditor is read-only, including the audit log.
func TestPredefinedRoles_GOV_002(t *testing.T) {
	t.Parallel()
	var names []string
	for _, r := range auth.PredefinedRoles() {
		names = append(names, string(r.Name))
		if r.Description == "" {
			t.Errorf("%s has no description", r.Name)
		}
	}
	want := []string{
		"owner", "admin", "release-manager", "developer", "designer", "translator",
		"reviewer", "compliance-officer", "auditor", "viewer",
	}
	if !slices.Equal(names, want) {
		t.Errorf("predefined roles = %v, want %v", names, want)
	}
	for _, r := range auth.Roles() {
		if !auth.IsPredefined(r) {
			t.Errorf("%s is listed and not predefined", r)
		}
		if _, err := auth.ParseRole(string(r)); err != nil {
			t.Errorf("ParseRole(%s): %v", r, err)
		}
	}
	if _, err := auth.ParseRole("wizard"); code(err) != plxerr.RoleUnknown {
		t.Errorf("an unknown role: %v", err)
	}

	auditor := auth.RolePermissions(auth.RoleAuditor)
	for _, p := range []auth.Permission{auth.AuditRead, auth.AuditExport, auth.ReconstructionView, auth.ComplianceReport} {
		if !slices.Contains(auditor, p) {
			t.Errorf("the auditor lacks %s", p)
		}
	}
	for _, p := range auditor {
		if info, _ := auth.Describe(p); !info.ReadOnly {
			t.Errorf("the auditor holds %s, which changes things", p)
		}
	}
	// Every role other than the owner is a subset of what its description
	// allows: none holds a permission that manages roles, identity or the
	// licence, which are the owner's.
	for _, r := range auth.Roles() {
		if r == auth.RoleOwner {
			continue
		}
		for _, p := range []auth.Permission{auth.RolesManage, auth.SSOManage, auth.SessionsManage, auth.LicenseManage, auth.BackupManage, auth.RetentionManage, auth.KeysManage, auth.MembersManage} {
			if slices.Contains(auth.RolePermissions(r), p) {
				t.Errorf("%s holds %s", r, p)
			}
		}
	}
	// The release manager publishes and promotes but does not edit code.
	rm := auth.RolePermissions(auth.RoleReleaseManager)
	if !slices.Contains(rm, auth.ReleasePublish) || !slices.Contains(rm, auth.ReleasePromote) || slices.Contains(rm, auth.PluginEdit) {
		t.Errorf("the release manager holds %v", rm)
	}
}

// Verifies: GOV-002, GOV-003.
// Scoped permissions parse as <permission>:<environment>; unknown
// permissions are PLX-8101 and an environment on a permission that cannot
// have one is PLX-8102.
func TestParseScopedPermission_GOV_003(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in   string
		want auth.ScopedPermission
		code plxerr.Code
	}{
		{"release.publish", auth.ScopedPermission{Permission: auth.ReleasePublish}, 0},
		{" release.publish:production ", auth.ScopedPermission{Permission: auth.ReleasePublish, Environment: "production"}, 0},
		{"secrets.manage:staging-2", auth.ScopedPermission{Permission: auth.SecretsManage, Environment: "staging-2"}, 0},
		{"ghosts.manage", auth.ScopedPermission{}, plxerr.PermissionNotInCatalogue},
		{"ghosts.manage:production", auth.ScopedPermission{}, plxerr.PermissionNotInCatalogue},
		{"audit.read:production", auth.ScopedPermission{}, plxerr.ScopeInvalid},
		{"app.read:production", auth.ScopedPermission{}, plxerr.ScopeInvalid},
		{"release.publish:", auth.ScopedPermission{}, plxerr.ScopeInvalid},
		{"release.publish:-x", auth.ScopedPermission{}, plxerr.ScopeInvalid},
		{"release.publish:Production", auth.ScopedPermission{}, plxerr.ScopeInvalid},
		{"", auth.ScopedPermission{}, plxerr.PermissionNotInCatalogue},
	} {
		got, err := auth.ParseScopedPermission(c.in)
		if code(err) != c.code || got != c.want {
			t.Errorf("ParseScopedPermission(%q) = %+v, %v; want %+v, code %v", c.in, got, err, c.want, c.code)
		}
	}
	if got := (auth.ScopedPermission{Permission: auth.ReleasePublish, Environment: "production"}).String(); got != "release.publish:production" {
		t.Errorf("String = %q", got)
	}
}

// Verifies: GOV-002.
// The published table lists every permission and every predefined role.
func TestPermissionsDocument_GOV_002(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile("../../../docs/reference/permissions.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	for _, e := range auth.Catalogue() {
		if !strings.Contains(doc, "| `"+string(e.Name)+"` | `"+e.Action+"` | "+e.Description) {
			t.Errorf("docs/reference/permissions.md lacks %s; run make gen", e.Name)
		}
	}
	for _, r := range auth.PredefinedRoles() {
		if !strings.Contains(doc, "**"+string(r.Name)+"**") {
			t.Errorf("docs/reference/permissions.md lacks the role %s; run make gen", r.Name)
		}
	}
}

// Verifies: GOV-002, GOV-006.
// The catalogue is grouped as area, feature, action without renaming
// anything: plugin.lock.override is area plugin, feature plugin, action
// lock.override, and restrictions.manage is a high-risk, step-up
// organisation permission held by Owner and Admin.
func TestCatalogueGrouping_GOV_002(t *testing.T) {
	t.Parallel()
	lock, ok := auth.Describe(auth.PluginLockOverride)
	if !ok || lock.Area != "plugin" || lock.Feature != "plugin" || lock.Action != "lock.override" {
		t.Errorf("plugin.lock.override = %+v", lock)
	}
	deploy, _ := auth.Describe(auth.FunctionDeployProduction)
	if deploy.Area != "function" || deploy.Action != "deploy.production" {
		t.Errorf("function.deploy.production = %+v", deploy)
	}
	r, ok := auth.Describe(auth.RestrictionsManage)
	if !ok || r.Risk != auth.RiskHigh || !r.StepUp || r.Scope != auth.ScopeOrganization || r.ReadOnly {
		t.Errorf("restrictions.manage = %+v", r)
	}
	for _, role := range []auth.Role{auth.RoleOwner, auth.RoleAdmin} {
		if !slices.Contains(auth.RolePermissions(role), auth.RestrictionsManage) {
			t.Errorf("%s does not hold restrictions.manage", role)
		}
	}
	for _, role := range []auth.Role{auth.RoleDeveloper, auth.RoleViewer, auth.RoleReleaseManager, auth.RoleAuditor} {
		if slices.Contains(auth.RolePermissions(role), auth.RestrictionsManage) {
			t.Errorf("%s holds restrictions.manage", role)
		}
	}
}
