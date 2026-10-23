// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"slices"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// AccessScope is where a permission is asked about: the whole organisation
// (both fields empty), one app, or one environment of an app, named by its
// key (GOV-003, GOV-007).
type AccessScope struct {
	AppID       string
	Environment string
}

// validate refuses an environment that names no app.
func (s AccessScope) validate() error {
	if s.Environment != "" && s.AppID == "" {
		return plxerr.New(plxerr.InvalidStructure, "an environment is named together with its app")
	}
	return nil
}

// EffectivePermissions is what a principal may do in a scope: one compact,
// sorted set, with the organisation's permissions version it was resolved at
// (GOV-007). A client refetches when WatchPermissions announces a newer
// version.
type EffectivePermissions struct {
	Permissions []Permission
	Version     int64
}

// Effective returns the principal's permissions in a scope: those held across
// the organisation, on the app and on the environment, less what a
// restriction removes there (GOV-006, GOV-007). The principal is already net
// of the restrictions on the whole organisation and of the token's scopes.
func (p Principal) Effective(scope AccessScope) (EffectivePermissions, error) {
	if err := scope.validate(); err != nil {
		return EffectivePermissions{}, err
	}
	set := slices.Clone(p.Permissions)
	if scope.AppID != "" {
		set = append(set, p.AppPermissions[scope.AppID]...)
	}
	if scope.Environment != "" {
		set = append(set, p.EnvPermissions[EnvScope(scope)]...)
		set = append(set, p.EnvPermissions[EnvScope{Environment: scope.Environment}]...)
	}
	set = without(set, func(perm Permission) bool { return p.restrictedOn(perm, scope.AppID, scope.Environment) })
	slices.Sort(set)
	return EffectivePermissions{Permissions: slices.Compact(set), Version: p.Version}, nil
}

// holdsIn reports whether the principal holds a permission in a scope,
// ignoring the second-factor rule.
func (p Principal) holdsIn(perm Permission, scope AccessScope) bool {
	switch {
	case scope.Environment != "":
		return p.HoldsOnEnvironment(perm, scope.AppID, scope.Environment)
	case scope.AppID != "":
		return p.HoldsOnApp(perm, scope.AppID)
	}
	return p.Holds(perm)
}
