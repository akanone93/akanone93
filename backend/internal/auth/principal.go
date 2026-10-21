// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"slices"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// EnvScope names the environments a permission applies to besides the
// whole organisation and whole apps: the environment with the given key
// of one app, or, with an empty AppID, of every app (GOV-003).
type EnvScope struct {
	AppID       string
	Environment string
}

// Principal is who is calling, resolved against one organisation.
type Principal struct {
	Identity
	// OrganizationID is the organisation this call acts in.
	OrganizationID string
	// Permissions are those held across the organisation, after a
	// token's scopes are applied.
	Permissions []Permission
	// AppPermissions are those held on single apps, by app ID, after a
	// token's scopes are applied (GOV-001).
	AppPermissions map[string][]Permission
	// EnvPermissions are those held on single environments, written
	// <permission>:<environment>, after a token's scopes are applied
	// (GOV-003).
	EnvPermissions map[EnvScope][]Permission
	// Restrictions are the restrictions that applied to this principal,
	// each with what it removed, so that a decision can be explained
	// (GOV-006, GOV-007). The permission sets above are already net of
	// them.
	Restrictions []AppliedRestriction
	// Version is the organisation's permissions version the sets above
	// were resolved at (GOV-007); zero for an identity resolved without
	// reading one.
	Version int64
}

// roleGrant is one grant as the resolver reads it: a role at a scope
// (GOV-003). AppID is set for app and environment scope, Environment (the
// environment's key) for environment scope.
type roleGrant struct {
	// ID identifies the grant, membership or app access entry it was read
	// from, for explaining a decision (GOV-007); "" for a proposal.
	ID          string
	Role        Role
	Scope       Scope
	AppID       string
	Environment string
}

// rolePermissions returns the permissions a role holds as scoped
// permissions: a predefined role's are bare, a custom role's are as it was
// composed. An unknown role holds nothing.
func rolePermissions(r Role, custom map[Role][]ScopedPermission) []ScopedPermission {
	if perms, ok := grants[r]; ok {
		out := make([]ScopedPermission, len(perms))
		for i, perm := range perms {
			out[i] = ScopedPermission{Permission: perm}
		}
		return out
	}
	return custom[r]
}

// resolve computes a principal's permissions from its grants, minus the
// restrictions that apply to it, intersected with a token's scopes when it
// has any. A token can therefore never do more than its user can do now:
// removing the user's role removes it from the token too.
//
// A restriction removes permissions whatever the grants say and however
// they combine: the grants are collected first, every restriction is
// subtracted, and only then does an Owner of the organisation get back the
// owner floor, so that no restriction can lock the organisation out of its
// own administration (GOV-006, ADR-0086).
//
// A grant carries a permission only where the permission can be held: an
// organisation permission only through a grant on the organisation, and an
// environment grant only the permissions that can be limited to an
// environment, and only on that environment. A permission written
// <permission>:<environment> in a role applies to the environment with that
// key, within the grant's scope.
func resolve(id Identity, organizationID string, given []roleGrant, custom map[Role][]ScopedPermission, direct []Permission, restrictions ...Restriction) Principal {
	p := Principal{
		Identity: id, OrganizationID: organizationID,
		AppPermissions: map[string][]Permission{}, EnvPermissions: map[EnvScope][]Permission{},
	}
	h := held{org: slices.Clone(direct), apps: map[string][]Permission{}, envs: map[EnvScope][]Permission{}}
	for _, g := range given {
		for _, sp := range rolePermissions(g.Role, custom) {
			if info, ok := Describe(sp.Permission); ok {
				h.add(g, sp, info.Scope)
			}
		}
	}
	p.Restrictions = h.applyRestrictions(restrictions, ownerFloorFor(given))
	org, apps, envs := h.org, h.apps, h.envs
	p.Permissions = restrict(org, id.Scopes)
	for app, held := range apps {
		if perms := restrict(held, id.Scopes); len(perms) > 0 {
			p.AppPermissions[app] = perms
		}
	}
	for scope, held := range envs {
		if perms := restrict(held, id.Scopes); len(perms) > 0 {
			p.EnvPermissions[scope] = perms
		}
	}
	return p
}

// held collects the permissions a principal's grants carry, by where they
// apply.
type held struct {
	org  []Permission
	apps map[string][]Permission
	envs map[EnvScope][]Permission
}

// add places one permission of a grant's role, whose catalogue scope is
// scope, where the grant lets it apply (see resolve).
func (h *held) add(g roleGrant, sp ScopedPermission, scope Scope) {
	switch g.Scope {
	case ScopeOrganization:
		if sp.Environment == "" {
			h.org = append(h.org, sp.Permission)
		} else if scope == ScopeEnvironment {
			key := EnvScope{Environment: sp.Environment}
			h.envs[key] = append(h.envs[key], sp.Permission)
		}
	case ScopeApp:
		switch {
		case scope == ScopeOrganization:
		case sp.Environment == "":
			h.apps[g.AppID] = append(h.apps[g.AppID], sp.Permission)
		case scope == ScopeEnvironment:
			key := EnvScope{AppID: g.AppID, Environment: sp.Environment}
			h.envs[key] = append(h.envs[key], sp.Permission)
		}
	case ScopeEnvironment:
		if scope == ScopeEnvironment && (sp.Environment == "" || sp.Environment == g.Environment) {
			key := EnvScope{AppID: g.AppID, Environment: g.Environment}
			h.envs[key] = append(h.envs[key], sp.Permission)
		}
	}
}

// restrict sorts and de-duplicates held. scopes is nil for a session,
// which is not restricted, and never nil for a token, whose scopes are
// fixed when it is created: a token with no scopes can do nothing.
func restrict(held, scopes []Permission) []Permission {
	out := make([]Permission, 0, len(held))
	for _, perm := range held {
		if scopes != nil && !slices.Contains(scopes, perm) {
			continue
		}
		out = append(out, perm)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Holds reports whether the principal holds a permission across the
// organisation, ignoring the second-factor rule.
func (p Principal) Holds(want Permission) bool { return slices.Contains(p.Permissions, want) }

// HoldsOnApp reports whether the principal holds a permission on an app,
// through the organisation or through a grant on that app, and no
// restriction removes it there.
func (p Principal) HoldsOnApp(want Permission, appID string) bool {
	return (p.Holds(want) || slices.Contains(p.AppPermissions[appID], want)) && !p.restrictedOn(want, appID, "")
}

// HoldsOnEnvironment reports whether the principal holds a permission on
// the environment with the given key of an app: across the organisation, on
// the app, or through a grant limited to that environment (GOV-003), and no
// restriction removes it there.
func (p Principal) HoldsOnEnvironment(want Permission, appID, environment string) bool {
	held := p.Holds(want) ||
		slices.Contains(p.AppPermissions[appID], want) ||
		slices.Contains(p.EnvPermissions[EnvScope{AppID: appID, Environment: environment}], want) ||
		slices.Contains(p.EnvPermissions[EnvScope{Environment: environment}], want)
	return held && !p.restrictedOn(want, appID, environment)
}

// HoldsInSomeEnvironment reports whether the principal holds a permission
// on an app or on at least one of its environments. It is for the first
// check of a call whose environment is not yet known; the call must then
// check the environment with AuthorizeEnvironment.
func (p Principal) HoldsInSomeEnvironment(want Permission, appID string) bool {
	if p.HoldsOnApp(want, appID) {
		return true
	}
	for scope, perms := range p.EnvPermissions {
		if (scope.AppID == appID || scope.AppID == "") && slices.Contains(perms, want) &&
			!p.restrictedOn(want, appID, scope.Environment) {
			return true
		}
	}
	return false
}

// restrictedOn reports whether a restriction limited to an app removes a
// permission on that app, or on one of its environments when environment is
// not empty. A restriction on the whole organisation is not asked here: it
// is already out of every permission set. A restriction on an environment
// does not touch a check of the app itself, which does not name one
// (GOV-006).
func (p Principal) restrictedOn(want Permission, appID, environment string) bool {
	return slices.ContainsFunc(p.Restrictions, func(r AppliedRestriction) bool {
		return r.AppID != "" && r.AppID == appID &&
			(r.Environment == "" || r.Environment == environment) &&
			slices.Contains(r.Removed, want)
	})
}

// Authorize returns nil when the principal may use a permission across
// the organisation, and otherwise the refusal. It is the only way a call
// is allowed: there is no default yes (SEC-102).
func (p Principal) Authorize(want Permission) error {
	return p.check(want, p.Holds(want))
}

// AuthorizeApp is Authorize for a permission on one app.
func (p Principal) AuthorizeApp(want Permission, appID string) error {
	return p.check(want, p.HoldsOnApp(want, appID))
}

// AuthorizeEnvironment is Authorize for a permission on one environment of
// an app, named by its key (GOV-003).
func (p Principal) AuthorizeEnvironment(want Permission, appID, environment string) error {
	return p.check(want, p.HoldsOnEnvironment(want, appID, environment))
}

// AuthorizeSomeEnvironment is the first check of a call whose environment
// is not yet known: it passes when the principal holds the permission on
// the app or on one of its environments. The call then checks the
// environment itself with AuthorizeEnvironment.
func (p Principal) AuthorizeSomeEnvironment(want Permission, appID string) error {
	return p.check(want, p.HoldsInSomeEnvironment(want, appID))
}

// check turns a decision into the error the edge reports.
func (p Principal) check(want Permission, held bool) error {
	if p.OrganizationID == "" {
		return plxerr.New(plxerr.AuthenticationRequired, "this call acts in an organisation, and none was named")
	}
	if !held {
		return plxerr.New(plxerr.PermissionDenied, "%s is required", want)
	}
	if !NeedsSecondFactor(want) {
		return nil
	}
	if !p.SecondFactor {
		return plxerr.New(plxerr.MultiFactorRequired, "%s requires a second factor in this session", want)
	}
	// The second factor of the sign-in is not enough: the permissions the
	// catalogue marks stepUp need a WebAuthn assertion with user
	// verification from the last five minutes, in the browser session or
	// through the CLI's grant (SEC-103, ADR-0075).
	return p.StepUpError(StepUpWindow)
}

// ExcludedApps returns the apps on which a restriction takes a permission
// away from a principal who holds it across the organisation, sorted. A
// listing that reads an organisation-wide permission as "every app" must
// leave these out; the permission sets of an app granted one by one are
// already net of restrictions (GOV-006).
func (p Principal) ExcludedApps(want Permission) []string {
	var out []string
	for _, r := range p.Restrictions {
		if r.AppID != "" && r.Environment == "" && slices.Contains(r.Removed, want) {
			out = append(out, r.AppID)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// AllowedApps returns nil when the principal may read every app, and
// otherwise the apps it was granted one by one. A principal that may read
// every app may still be barred from some by restrictions; those are
// ExcludedApps, and a listing leaves them out (GOV-006).
func (p Principal) AllowedApps() []string {
	if p.Holds(AppRead) {
		return nil
	}
	out := make([]string, 0, len(p.AppPermissions))
	for app, perms := range p.AppPermissions {
		if slices.Contains(perms, AppRead) {
			out = append(out, app)
		}
	}
	slices.Sort(out)
	return out
}
