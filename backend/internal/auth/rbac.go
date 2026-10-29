// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Permission is one entry of the permission catalogue. Every call names
// the permission it needs; a principal that does not hold it is refused
// (SEC-102). The catalogue, with each permission's description, risk,
// step-up flag and scope, is permissions.json (GOV-002); a test keeps the
// constants below and in rbac_p9.go equal to it.
type Permission string

// The permissions of phases 2 to 8; those of phase 9 are in rbac_p9.go.
const (
	OrganizationRead   Permission = "organization.read"
	OrganizationManage Permission = "organization.manage"
	MembersManage      Permission = "members.manage"
	AppRead            Permission = "app.read"
	AppManage          Permission = "app.manage"
	EnvironmentManage  Permission = "environment.manage"
	SecretsManage      Permission = "secrets.manage"
	LimitsManage       Permission = "limits.manage"
	PluginRead         Permission = "plugin.read"
	PluginEdit         Permission = "plugin.edit"
	PluginLockOverride Permission = "plugin.lock.override"
	ReleasePublish     Permission = "release.publish"
	ReleasePromote     Permission = "release.promote"
	TokensManage       Permission = "tokens.manage"
	KeysManage         Permission = "keys.manage"
	AuditRead          Permission = "audit.read"
	TrashManage        Permission = "trash.manage"
	// FunctionRead reads functions, their versions and builds (FN-006).
	FunctionRead Permission = "function.read"
	// FunctionDeploy uploads, builds and deploys functions outside
	// production (FN-006).
	FunctionDeploy Permission = "function.deploy"
	// FunctionDeployProduction deploys a function to a production
	// environment (FN-007). It needs a fresh step-up with a security key
	// (GOV-005, SEC-103); the approval engine (GOV-020) adds a second person.
	FunctionDeployProduction Permission = "function.deploy.production"
	// TranslationsEdit saves the messages and states of translations
	// documents, under the per-locale lock (I18N-008).
	TranslationsEdit Permission = "translations.edit"
	// TranslationsApprove sets or keeps a translated message at the
	// approved state (I18N-008). It requires a second factor.
	TranslationsApprove Permission = "translations.approve"
	// A11yWaive waives an accessibility finding (ADR-0069). It requires
	// a second factor.
	A11yWaive Permission = "a11y.waive"
	// GoldensApprove approves a golden-image change (ADR-0069). It
	// requires a second factor.
	GoldensApprove Permission = "goldens.approve"
	// LibraryManage edits the organisation's shared templates and components
	// that someone else made (ADR-0098).
	LibraryManage Permission = "library.manage"
	// LibraryPublish makes a template or component public to the whole
	// installation (ADR-0098, STU-081).
	LibraryPublish Permission = "library.publish"
	// AnalyticsManage creates, changes and deletes funnel definitions
	// (ANL-020).
	AnalyticsManage Permission = "analytics.manage"
)

// allPermissions is every permission of the catalogue, sorted.
var allPermissions = catalogueNames()

// catalogueNames lists the names of the generated catalogue.
func catalogueNames() []Permission {
	out := make([]Permission, len(catalogue))
	for i, e := range catalogue {
		out[i] = e.Name
	}
	return out
}

// Permissions returns the catalogue's permissions, in order.
func Permissions() []Permission { return slices.Clone(allPermissions) }

// Role is a named set of permissions. The predefined roles are those of
// GOV-002; the grants of each are generated from roles.json. An
// organisation's custom roles (GOV-002) are Roles too, named by their key,
// and resolve through the database.
type Role string

// The predefined roles. Their permissions are in roles.json.
const (
	// RoleOwner may do everything, including managing members and keys.
	RoleOwner Role = "owner"
	// RoleAdmin may do everything except manage members and keys.
	RoleAdmin Role = "admin"
	// RoleReleaseManager may publish, promote and roll back, but not edit
	// code.
	RoleReleaseManager Role = "release-manager"
	// RoleDeveloper may edit plugins and publish to development, but not
	// promote, manage secrets or read the audit log.
	RoleDeveloper Role = "developer"
	// RoleDesigner may edit plugins and translations, and not publish.
	RoleDesigner Role = "designer"
	// RoleTranslator may read and edit translations, and nothing else
	// (I18N-008).
	RoleTranslator Role = "translator"
	// RoleReviewer may read and decide on approvals.
	RoleReviewer Role = "reviewer"
	// RoleComplianceOfficer may read the audit log, manage policies and
	// freezes, and produce compliance reports.
	RoleComplianceOfficer Role = "compliance-officer"
	// RoleAuditor may read everything, including the audit log, and
	// export it.
	RoleAuditor Role = "auditor"
	// RoleViewer may read.
	RoleViewer Role = "viewer"
)

// Roles returns every predefined role, in name order.
func Roles() []Role {
	out := make([]Role, 0, len(grants))
	for r := range grants {
		out = append(out, r)
	}
	slices.Sort(out)
	return out
}

// IsPredefined reports whether a name is a predefined role.
func IsPredefined(r Role) bool {
	_, ok := grants[r]
	return ok
}

// ParseRole reads a predefined role's name, refusing anything that is not
// one with PLX-8100.
func ParseRole(s string) (Role, error) {
	r := Role(s)
	if !IsPredefined(r) {
		names := make([]string, len(predefined))
		for i, info := range predefined {
			names[i] = string(info.Name)
		}
		return "", plxerr.New(plxerr.RoleUnknown, "%q is not a role; the roles are %s", s, strings.Join(names, ", "))
	}
	return r, nil
}

// RolePermissions returns the permissions a predefined role holds, in order.
func RolePermissions(r Role) []Permission { return slices.Clone(grants[r]) }

// NeedsSecondFactor reports whether a permission may be used only after
// a second factor (SEC-100, SEC-103); the catalogue says which do.
func NeedsSecondFactor(p Permission) bool {
	info, ok := Describe(p)
	return ok && info.StepUp
}

// Actor is how the audit log records the principal.
func (p Principal) Actor() audit.Actor { return p.Identity.Actor() }

// System is the principal of work the server does for itself, such as
// purging the trash. It holds everything and is audited as "system".
func System(organizationID string) Principal {
	return Principal{
		Identity:       Identity{Kind: KindSystem, ID: "server", Display: "Plux", SecondFactor: true},
		OrganizationID: organizationID,
		Permissions:    Permissions(),
		AppPermissions: map[string][]Permission{},
		EnvPermissions: map[EnvScope][]Permission{},
	}
}

// ParseScopes reads the scopes of a token, refusing anything outside
// the catalogue so that a typo cannot widen a token silently.
func ParseScopes(values []string) ([]Permission, error) {
	out := make([]Permission, 0, len(values))
	for _, v := range values {
		p := Permission(strings.TrimSpace(v))
		if !InCatalogue(p) {
			return nil, plxerr.New(plxerr.PermissionNotInCatalogue, "%q is not a permission; see the catalogue", v)
		}
		out = append(out, p)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// ScopeStrings renders scopes for storage and for responses.
func ScopeStrings(scopes []Permission) []string {
	out := make([]string, len(scopes))
	for i, s := range scopes {
		out[i] = string(s)
	}
	return out
}

// subset reports the first scope a principal does not hold across the
// organisation, so that no one can mint a token that does more than
// they can (SRV-064).
func subset(p Principal, scopes []Permission) error {
	for _, s := range scopes {
		if !p.Holds(s) {
			return plxerr.New(plxerr.PermissionDenied, "a token cannot carry %s, which you do not hold", s)
		}
	}
	return nil
}
