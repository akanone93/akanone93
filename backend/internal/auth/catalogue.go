// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

//go:generate go run ./internal/gen permissions.json roles.json catalogue_gen.go ../../../docs/reference/permissions.md

import (
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Risk is how much damage a misused permission can do (GOV-002).
type Risk string

// The risks, from least to most.
const (
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
	RiskHigh   Risk = "high"
)

// Scope is a level of the hierarchy a permission can be granted at, and
// the level a grant applies to (GOV-003). For a permission it is the
// narrowest level it can be granted at: an organisation permission applies
// across the organisation only, an app permission can also be granted on
// one app, an environment permission also on one environment of an app.
type Scope string

// The scopes, from widest to narrowest.
const (
	ScopeOrganization Scope = "organization"
	ScopeApp          Scope = "app"
	ScopeEnvironment  Scope = "environment"
)

// PermissionInfo describes one permission of the catalogue. The catalogue
// is generated from permissions.json, which is its single source
// (ADR-0073).
type PermissionInfo struct {
	Name        Permission
	Description string
	// Area, Feature and Action group the catalogue as area, feature,
	// action (GOV-002, ADR-0086). Names are not renamed: for
	// plugin.lock.override the area and the feature are plugin and the
	// action is lock.override.
	Area, Feature, Action string
	Risk                  Risk
	// StepUp is true when the permission may be used only after a second
	// factor (SEC-100, SEC-103).
	StepUp bool
	// Scope is the narrowest scope the permission can be granted at.
	Scope Scope
	// ReadOnly is true when the permission changes nothing.
	ReadOnly bool
}

// RoleInfo describes a predefined role.
type RoleInfo struct {
	Name        Role
	Description string
}

// Catalogue returns every permission with its description, risk, step-up
// flag and scope, sorted by name (GOV-002).
func Catalogue() []PermissionInfo { return slices.Clone(catalogue) }

// Describe returns the catalogue entry of a permission.
func Describe(p Permission) (PermissionInfo, bool) {
	i, found := slices.BinarySearchFunc(catalogue, p, func(e PermissionInfo, want Permission) int {
		return strings.Compare(string(e.Name), string(want))
	})
	if !found {
		return PermissionInfo{}, false
	}
	return catalogue[i], true
}

// InCatalogue reports whether a permission is in the catalogue.
func InCatalogue(p Permission) bool {
	_, ok := Describe(p)
	return ok
}

// PredefinedRoles returns the predefined roles in the order the
// specification lists them (GOV-002).
func PredefinedRoles() []RoleInfo { return slices.Clone(predefined) }

// ParsePermission reads a permission name, refusing anything outside the
// catalogue with PLX-8101.
func ParsePermission(s string) (Permission, error) {
	p := Permission(strings.TrimSpace(s))
	if !InCatalogue(p) {
		return "", plxerr.New(plxerr.PermissionNotInCatalogue, "%q is not a permission; see the catalogue", s)
	}
	return p, nil
}

// ScopedPermission is a permission as a role holds it: bare, or limited to
// the environment with the given key, written <permission>:<environment>
// (GOV-002, GOV-003).
type ScopedPermission struct {
	Permission Permission
	// Environment is the key of the environment the permission is limited
	// to; "" for a bare permission.
	Environment string
}

// String renders the permission as it is written in a role.
func (s ScopedPermission) String() string {
	if s.Environment == "" {
		return string(s.Permission)
	}
	return string(s.Permission) + ":" + s.Environment
}

// ParseScopedPermission reads "<permission>" or "<permission>:<environment>".
// A permission outside the catalogue is PLX-8101; an environment on a
// permission that cannot be limited to one is PLX-8102.
func ParseScopedPermission(s string) (ScopedPermission, error) {
	name, env, scoped := strings.Cut(strings.TrimSpace(s), ":")
	p, err := ParsePermission(name)
	if err != nil {
		return ScopedPermission{}, err
	}
	if !scoped {
		return ScopedPermission{Permission: p}, nil
	}
	if info, _ := Describe(p); info.Scope != ScopeEnvironment {
		return ScopedPermission{}, plxerr.New(plxerr.ScopeInvalid, "%s cannot be limited to an environment", p)
	}
	if !validKey(env) {
		return ScopedPermission{}, plxerr.New(plxerr.ScopeInvalid, "%q is not an environment key", env)
	}
	return ScopedPermission{Permission: p, Environment: env}, nil
}

// validKey reports whether s is shaped like an app, team or environment
// key: lower-case letters, digits and hyphens, not starting or ending with
// a hyphen, at most 64 characters.
func validKey(s string) bool {
	if s == "" || len(s) > 64 || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}
