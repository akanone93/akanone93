// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"slices"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// SubjectKind is whom a restriction applies to (GOV-006).
type SubjectKind string

// The subjects of a restriction.
const (
	// SubjectOrganization restricts everyone in the organisation.
	SubjectOrganization SubjectKind = "organization"
	// SubjectTeam restricts every member of a team.
	SubjectTeam SubjectKind = "team"
	// SubjectUser restricts one person.
	SubjectUser SubjectKind = "user"
)

// Restriction is a ceiling over grants: it removes permissions from its
// subject whatever their grants say (GOV-006, ADR-0086). It is immutable; a
// change is a delete and a create.
type Restriction struct {
	ID string
	// SubjectKind says whom it applies to; SubjectID is the team or the
	// user, and "" for the organisation.
	SubjectKind SubjectKind
	SubjectID   string
	// Patterns name what it removes: catalogue permissions or the
	// wildcards "*", "<area>.*" and "<area>.<feature>.*".
	Patterns []string
	// AppID limits it to one app, and Environment, the key of one of its
	// environments, to that environment; both "" for the whole
	// organisation. An environment needs an app.
	AppID       string
	Environment string
	Reason      string
	// CreatedBy is the person who made it, "" when that person is gone.
	CreatedBy string
	CreatedAt time.Time
}

// AppliedRestriction is a restriction that applied to a principal, with the
// permissions it actually took from them. Explaining a decision reads it
// (GOV-007).
type AppliedRestriction struct {
	Restriction
	// Removed are the permissions the principal's grants carried and this
	// restriction removed, sorted. The owner floor is not among them.
	Removed []Permission
}

// ownerFloor are the permissions an Owner always keeps, so that no
// restriction can lock an organisation out of administering itself
// (GOV-006, ADR-0086).
var ownerFloor = []Permission{MembersManage, RolesManage, RestrictionsManage, OrganizationManage}

// OwnerFloor returns the permissions an Owner keeps whatever restrictions
// say: the ability to manage members, roles, restrictions and the
// organisation.
func OwnerFloor() []Permission { return slices.Clone(ownerFloor) }

// matchesPattern reports whether a pattern names a catalogue entry: the
// entry itself, "*", "<area>.*" for its area or "<area>.<feature>.*" for its
// feature (GOV-002, GOV-006).
func matchesPattern(pattern string, info PermissionInfo) bool {
	if pattern == "*" || pattern == string(info.Name) {
		return true
	}
	group, wildcard := strings.CutSuffix(pattern, ".*")
	return wildcard && (group == info.Area || group == info.Area+"."+info.Feature)
}

// matchedByPatterns returns the catalogue entries any of the patterns names.
func matchedByPatterns(patterns []string) []PermissionInfo {
	var out []PermissionInfo
	for _, info := range catalogue {
		if slices.ContainsFunc(patterns, func(p string) bool { return matchesPattern(p, info) }) {
			out = append(out, info)
		}
	}
	return out
}

// checkPatterns validates the patterns of a restriction against the
// catalogue: at least one, each naming at least one permission (PLX-8104).
func checkPatterns(patterns []string) ([]string, error) {
	if len(patterns) == 0 {
		return nil, plxerr.New(plxerr.RestrictionPatternInvalid, "a restriction names at least one permission or wildcard")
	}
	out := make([]string, 0, len(patterns))
	for _, raw := range patterns {
		pattern := strings.TrimSpace(raw)
		if !slices.ContainsFunc(catalogue, func(info PermissionInfo) bool { return matchesPattern(pattern, info) }) {
			return nil, plxerr.New(plxerr.RestrictionPatternInvalid,
				"%q names no permission of the catalogue; use a permission, *, <area>.* or <area>.<feature>.*", raw)
		}
		out = append(out, pattern)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// removes reports whether the restriction takes a permission away. A
// restriction limited to an app or an environment touches only permissions
// that can be exercised on an app: an organisation permission, such as
// managing members, is not on any app.
func (r Restriction) removes(perm Permission) bool {
	info, ok := Describe(perm)
	if !ok {
		return false
	}
	if r.AppID != "" && info.Scope == ScopeOrganization {
		return false
	}
	return slices.ContainsFunc(r.Patterns, func(p string) bool { return matchesPattern(p, info) })
}

// ownerFloorFor returns the owner floor when the grants make the person an
// Owner of the organisation, and nothing otherwise. An Owner of one app is
// not an Owner of the organisation.
func ownerFloorFor(given []roleGrant) []Permission {
	if slices.ContainsFunc(given, func(g roleGrant) bool { return g.Role == RoleOwner && g.Scope == ScopeOrganization }) {
		return ownerFloor
	}
	return nil
}

// without returns the permissions for which drop is false.
func without(perms []Permission, drop func(Permission) bool) []Permission {
	out := make([]Permission, 0, len(perms))
	for _, perm := range perms {
		if !drop(perm) {
			out = append(out, perm)
		}
	}
	return out
}

// removeFrom takes what the restriction removes out of the sets of grants
// it covers: everything for a restriction on the organisation, the app's
// permissions and those of its environments for one on an app, and one
// environment's for one on an environment. A grant on every app's
// environment of a key is left alone by a restriction on one app; the
// principal's lookups apply that restriction (see Principal.restrictedOn).
func (h *held) removeFrom(r Restriction) {
	drop := r.removes
	switch {
	case r.AppID == "":
		h.org = without(h.org, drop)
		for app, perms := range h.apps {
			h.apps[app] = without(perms, drop)
		}
		for scope, perms := range h.envs {
			h.envs[scope] = without(perms, drop)
		}
	case r.Environment == "":
		h.apps[r.AppID] = without(h.apps[r.AppID], drop)
		for scope, perms := range h.envs {
			if scope.AppID == r.AppID {
				h.envs[scope] = without(perms, drop)
			}
		}
	default:
		scope := EnvScope{AppID: r.AppID, Environment: r.Environment}
		h.envs[scope] = without(h.envs[scope], drop)
	}
}

// every returns each permission the sets carry, once.
func (h *held) every() []Permission {
	all := slices.Clone(h.org)
	for _, perms := range h.apps {
		all = append(all, perms...)
	}
	for _, perms := range h.envs {
		all = append(all, perms...)
	}
	slices.Sort(all)
	return slices.Compact(all)
}

// applyRestrictions subtracts the restrictions from what the grants carry
// and then gives an Owner the owner floor back (GOV-006, ADR-0086). It
// returns what each restriction removed. Deny wins: nothing a grant says
// brings a removed permission back, whichever grant or restriction came
// first. The restrictions are the ones that apply to the person, their
// teams and the organisation; the caller has already selected them.
func (h *held) applyRestrictions(restrictions []Restriction, floor []Permission) []AppliedRestriction {
	carried := h.every()
	var applied []AppliedRestriction
	for _, r := range restrictions {
		removed := without(carried, func(perm Permission) bool { return !r.removes(perm) || slices.Contains(floor, perm) })
		applied = append(applied, AppliedRestriction{Restriction: r, Removed: removed})
		h.removeFrom(r)
	}
	h.org = append(h.org, floor...)
	return applied
}
