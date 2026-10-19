// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// RoleDefinition is a role and the permissions it holds (GOV-002).
type RoleDefinition struct {
	// Key names the role: a predefined role's name, or a custom role's key.
	Key         string
	Name        string
	Description string
	Predefined  bool
	// Permissions are catalogue names, bare or limited to an environment
	// as <permission>:<environment>, sorted.
	Permissions []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RoleGrantRecord is a role bound to a user or a team at a scope
// (GOV-003).
type RoleGrantRecord struct {
	ID     string
	UserID string
	TeamID string
	Role   string
	Scope  Scope
	AppID  string
	// EnvironmentID is set for environment scope.
	EnvironmentID string
	CreatedAt     time.Time
}

// GrantSpec is what a new grant binds.
type GrantSpec struct {
	// UserID or TeamID, exactly one, is the holder.
	UserID, TeamID string
	Role           string
	Scope          Scope
	AppID          string
	EnvironmentID  string
}

// GrantFilter narrows a list of grants.
type GrantFilter struct {
	UserID, TeamID, AppID string
}

// customRolesEnabled reports whether this installation may create and
// change custom roles, an enterprise feature (GOV-002, GOV-033, ADR-0073,
// ADR-0084). The licence provider in ctx answers, which the server's
// interceptors put there; without one it is the community edition and the
// answer is no. Options.CustomRolesEnabled overrides it in tests. Custom
// roles that already exist keep resolving either way, so a lapsed licence
// never changes anyone's access.
func (s *Service) customRolesEnabled(ctx context.Context) bool {
	if s.o.CustomRolesEnabled != nil {
		return s.o.CustomRolesEnabled()
	}
	return edition.EnabledForWrite(ctx, edition.FeatureCustomRoles)
}

// customRoleTable reads the custom roles the given grants use, so that the
// resolver can expand them. Nothing is read when every grant is of a
// predefined role, which is the usual case.
func (*Service) customRoleTable(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, given []roleGrant) (map[Role][]ScopedPermission, error) {
	if !slices.ContainsFunc(given, func(g roleGrant) bool { return !IsPredefined(g.Role) }) {
		return nil, nil
	}
	rows, err := q.ListAllCustomRoles(ctx, org)
	if err != nil {
		return nil, fmt.Errorf("auth: read custom roles: %w", err)
	}
	out := make(map[Role][]ScopedPermission, len(rows))
	for _, row := range rows {
		perms := make([]ScopedPermission, 0, len(row.Permissions))
		for _, s := range row.Permissions {
			// A permission the catalogue no longer has is not held.
			if sp, err := ParseScopedPermission(s); err == nil {
				perms = append(perms, sp)
			}
		}
		out[Role(row.Key)] = perms
	}
	return out, nil
}

// predefinedDefinition is the definition of a predefined role.
func predefinedDefinition(info RoleInfo) RoleDefinition {
	perms := make([]string, len(grants[info.Name]))
	for i, p := range grants[info.Name] {
		perms[i] = string(p)
	}
	slices.Sort(perms)
	return RoleDefinition{Key: string(info.Name), Name: string(info.Name), Description: info.Description, Predefined: true, Permissions: perms}
}

func customDefinition(row dbgen.CustomRole) RoleDefinition {
	return RoleDefinition{
		Key: row.Key, Name: row.Name, Description: row.Description, Permissions: slices.Clone(row.Permissions),
		CreatedAt: storage.Time(row.CreatedAt), UpdatedAt: storage.Time(row.UpdatedAt),
	}
}

// ListRoles lists the predefined roles and the organisation's custom
// roles in key order, after the given key (GOV-002).
func (s *Service) ListRoles(ctx context.Context, p Principal, afterKey string, limit int32) ([]RoleDefinition, error) {
	if err := p.Authorize(OrganizationRead); err != nil {
		return nil, err
	}
	var out []RoleDefinition
	for _, info := range predefined {
		if string(info.Name) > afterKey {
			out = append(out, predefinedDefinition(info))
		}
	}
	err := s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListCustomRoles(ctx, dbgen.ListCustomRolesParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), AfterKey: afterKey, PageSize: limit,
		})
		if err != nil {
			return fmt.Errorf("auth: list roles: %w", err)
		}
		for _, row := range rows {
			out = append(out, customDefinition(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b RoleDefinition) int { return strings.Compare(a.Key, b.Key) })
	if int(limit) < len(out) {
		out = out[:limit]
	}
	return out, nil
}

// GetRole returns one role: PLX-8100 when there is none.
func (s *Service) GetRole(ctx context.Context, p Principal, key string) (RoleDefinition, error) {
	if err := p.Authorize(OrganizationRead); err != nil {
		return RoleDefinition{}, err
	}
	for _, info := range predefined {
		if string(info.Name) == key {
			return predefinedDefinition(info), nil
		}
	}
	var out RoleDefinition
	err := s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetCustomRole(ctx, dbgen.GetCustomRoleParams{OrganizationID: storage.MustUUID(p.OrganizationID), Key: key})
		if err != nil {
			return roleNotFound(err, key)
		}
		out = customDefinition(row)
		return nil
	})
	return out, err
}

// roleNotFound turns "no rows" into PLX-8100.
func roleNotFound(err error, key string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return plxerr.New(plxerr.RoleUnknown, "%q is not a role of this organisation", key)
	}
	return fmt.Errorf("auth: read role: %w", err)
}

// checkRoleText validates the text fields of a custom role.
func checkRoleText(name, description string) error {
	if name == "" || len(name) > 100 {
		return plxerr.New(plxerr.InvalidFormat, "a role's name is 1 to 100 characters")
	}
	if len(description) > 500 {
		return plxerr.New(plxerr.InvalidFormat, "a role's description is at most 500 characters")
	}
	return nil
}

// composeRole parses the permissions of a custom role, refusing any outside
// the catalogue (PLX-8101) or any the creator does not hold (a role can
// never grant more than its creator has), and returns them sorted.
func composeRole(p Principal, values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, plxerr.New(plxerr.InvalidStructure, "a role holds at least one permission")
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		sp, err := ParseScopedPermission(v)
		if err != nil {
			return nil, err
		}
		if !p.Holds(sp.Permission) {
			return nil, plxerr.New(plxerr.PermissionDenied, "a role cannot hold %s, which you do not hold", sp.Permission)
		}
		out = append(out, sp.String())
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// requireCustomRoles refuses when custom roles are not licensed.
func (s *Service) requireCustomRoles(ctx context.Context) error {
	if !s.customRolesEnabled(ctx) {
		return plxerr.New(plxerr.EnterpriseFeatureNotLicensed, "custom roles are an enterprise feature; this installation is not licensed for them")
	}
	return nil
}

// CreateRole creates a custom role (GOV-002). It needs roles.manage, the
// enterprise feature, and holds only permissions the caller holds.
func (s *Service) CreateRole(ctx context.Context, p Principal, key, name, description string, permissions []string) (RoleDefinition, error) {
	if err := p.Authorize(RolesManage); err != nil {
		return RoleDefinition{}, err
	}
	if err := s.requireCustomRoles(ctx); err != nil {
		return RoleDefinition{}, err
	}
	name, description = strings.TrimSpace(name), strings.TrimSpace(description)
	if !validKey(key) || key[0] < 'a' || key[0] > 'z' {
		return RoleDefinition{}, plxerr.New(plxerr.InvalidFormat, "a role's key is lower-case letters, digits and hyphens, starting with a letter")
	}
	if IsPredefined(Role(key)) {
		return RoleDefinition{}, plxerr.New(plxerr.ResourceExists, "%q is a predefined role", key)
	}
	if err := checkRoleText(name, description); err != nil {
		return RoleDefinition{}, err
	}
	perms, err := composeRole(p, permissions)
	if err != nil {
		return RoleDefinition{}, err
	}
	id, err := s.newID()
	if err != nil {
		return RoleDefinition{}, err
	}
	var out RoleDefinition
	err = s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		creator, err := storage.NullUUID(p.UserID)
		if err != nil {
			return fmt.Errorf("auth: %w", err)
		}
		row, err := dbgen.New(tx).CreateCustomRole(ctx, dbgen.CreateCustomRoleParams{
			ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(p.OrganizationID),
			Key: key, Name: name, Description: description, Permissions: perms, CreatedBy: creator,
		})
		if err != nil {
			if uniqueViolation(err) {
				return plxerr.New(plxerr.ResourceExists, "a role with the key %q exists", key)
			}
			return fmt.Errorf("auth: create a role: %w", err)
		}
		out = customDefinition(row)
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: p.OrganizationID, Actor: p.Actor(),
			Action: audit.RoleCreated, TargetKind: "role", TargetID: key, Detail: strings.Join(perms, " "),
		})
	})
	return out, err
}

// UpdateRole replaces the name, description and permissions of a custom
// role. The new permissions are held by the caller, and the change takes
// effect for every holder of the role at their next call.
func (s *Service) UpdateRole(ctx context.Context, p Principal, key, name, description string, permissions []string) (RoleDefinition, error) {
	if err := p.Authorize(RolesManage); err != nil {
		return RoleDefinition{}, err
	}
	if err := s.requireCustomRoles(ctx); err != nil {
		return RoleDefinition{}, err
	}
	if IsPredefined(Role(key)) {
		return RoleDefinition{}, plxerr.New(plxerr.PreconditionFailed, "%q is a predefined role and cannot be changed", key)
	}
	name, description = strings.TrimSpace(name), strings.TrimSpace(description)
	if err := checkRoleText(name, description); err != nil {
		return RoleDefinition{}, err
	}
	perms, err := composeRole(p, permissions)
	if err != nil {
		return RoleDefinition{}, err
	}
	var out RoleDefinition
	err = s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).UpdateCustomRole(ctx, dbgen.UpdateCustomRoleParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), Key: key, Name: name, Description: description, Permissions: perms,
		})
		if err != nil {
			return roleNotFound(err, key)
		}
		out = customDefinition(row)
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: p.OrganizationID, Actor: p.Actor(),
			Action: audit.RoleUpdated, TargetKind: "role", TargetID: key, Detail: strings.Join(perms, " "),
		})
	})
	return out, err
}

// DeleteRole deletes a custom role that no grant uses. It is not an
// enterprise-gated act: a lapsed licence never traps a role.
func (s *Service) DeleteRole(ctx context.Context, p Principal, key string) error {
	if err := p.Authorize(RolesManage); err != nil {
		return err
	}
	if IsPredefined(Role(key)) {
		return plxerr.New(plxerr.PreconditionFailed, "%q is a predefined role and cannot be deleted", key)
	}
	return s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		org := storage.MustUUID(p.OrganizationID)
		if _, err := q.GetCustomRole(ctx, dbgen.GetCustomRoleParams{OrganizationID: org, Key: key}); err != nil {
			return roleNotFound(err, key)
		}
		used, err := q.CountGrantsOfRole(ctx, dbgen.CountGrantsOfRoleParams{OrganizationID: org, Role: key})
		if err != nil {
			return fmt.Errorf("auth: count the grants of a role: %w", err)
		}
		if used > 0 {
			return plxerr.New(plxerr.PreconditionFailed, "the role %q is granted %d times; revoke those grants first", key, used)
		}
		if _, err := q.DeleteCustomRole(ctx, dbgen.DeleteCustomRoleParams{OrganizationID: org, Key: key}); err != nil {
			return fmt.Errorf("auth: delete a role: %w", err)
		}
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: p.OrganizationID, Actor: p.Actor(),
			Action: audit.RoleDeleted, TargetKind: "role", TargetID: key,
		})
	})
}

// roleHeld resolves the permissions of a role of the organisation, refusing
// an unknown role with PLX-8100, and refuses a role holding a permission
// the caller does not hold across the organisation: nobody grants more than
// they have.
func roleHeld(ctx context.Context, q *dbgen.Queries, p Principal, role string) error {
	var perms []ScopedPermission
	if IsPredefined(Role(role)) {
		perms = rolePermissions(Role(role), nil)
	} else {
		row, err := q.GetCustomRole(ctx, dbgen.GetCustomRoleParams{OrganizationID: storage.MustUUID(p.OrganizationID), Key: role})
		if err != nil {
			return roleNotFound(err, role)
		}
		for _, v := range row.Permissions {
			if sp, err := ParseScopedPermission(v); err == nil {
				perms = append(perms, sp)
			}
		}
	}
	for _, sp := range perms {
		if !p.Holds(sp.Permission) {
			return plxerr.New(plxerr.PermissionDenied, "the role %q holds %s, which you do not hold, so you cannot grant it", role, sp.Permission)
		}
	}
	return nil
}

func grantRecord(row dbgen.RoleGrant) RoleGrantRecord {
	return RoleGrantRecord{
		ID: storage.ID(row.ID), UserID: storage.ID(row.UserID), TeamID: storage.ID(row.TeamID),
		Role: row.Role, Scope: Scope(row.Scope), AppID: storage.ID(row.AppID),
		EnvironmentID: storage.ID(row.EnvironmentID), CreatedAt: storage.Time(row.CreatedAt),
	}
}

// ListGrants lists the grants made with CreateGrant, in identifier order.
func (s *Service) ListGrants(ctx context.Context, p Principal, f GrantFilter, after storage.Cursor, limit int32) ([]RoleGrantRecord, error) {
	if err := p.Authorize(OrganizationRead); err != nil {
		return nil, err
	}
	user, err := storage.NullUUID(f.UserID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the user identifier is not valid")
	}
	team, err := storage.NullUUID(f.TeamID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the team identifier is not valid")
	}
	app, err := storage.NullUUID(f.AppID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	var out []RoleGrantRecord
	err = s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListRoleGrants(ctx, dbgen.ListRoleGrantsParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), AfterID: after.AfterID(),
			UserID: user, TeamID: team, AppID: app, PageSize: limit,
		})
		if err != nil {
			return fmt.Errorf("auth: list grants: %w", err)
		}
		out = make([]RoleGrantRecord, len(rows))
		for i, row := range rows {
			out[i] = grantRecord(row)
		}
		return nil
	})
	return out, err
}

// checkScope validates the scope of a new grant and returns its app and
// environment: the scope names an app, or an environment of that app, of
// the organisation (PLX-8102).
func checkScope(ctx context.Context, q *dbgen.Queries, spec GrantSpec) (app, environment pgtype.UUID, err error) {
	bad := func(msg string) error { return plxerr.New(plxerr.ScopeInvalid, "%s", msg) }
	if err := scopeShape(spec); err != nil || spec.Scope == ScopeOrganization {
		return app, environment, err
	}
	if app, err = storage.UUID(spec.AppID); err != nil {
		return app, environment, bad("the app identifier is not valid")
	}
	row, err := q.GetApp(ctx, app)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app, environment, bad("the organisation has no such app")
		}
		return app, environment, fmt.Errorf("auth: read an app: %w", err)
	}
	if spec.Scope == ScopeApp {
		return row.ID, environment, nil
	}
	if environment, err = storage.UUID(spec.EnvironmentID); err != nil {
		return app, environment, bad("the environment identifier is not valid")
	}
	env, err := q.GetEnvironment(ctx, environment)
	if err != nil || env.AppID != row.ID {
		if err == nil || errors.Is(err, pgx.ErrNoRows) {
			return app, environment, bad("the app has no such environment")
		}
		return app, environment, fmt.Errorf("auth: read an environment: %w", err)
	}
	return row.ID, env.ID, nil
}

// scopeShape checks that a grant names the app and environment its scope
// needs, and no more (PLX-8102).
func scopeShape(spec GrantSpec) error {
	var msg string
	switch spec.Scope {
	case ScopeOrganization:
		if spec.AppID != "" || spec.EnvironmentID != "" {
			msg = "a grant on the organisation names no app or environment"
		}
	case ScopeApp:
		if spec.AppID == "" || spec.EnvironmentID != "" {
			msg = "a grant on an app names the app and no environment"
		}
	case ScopeEnvironment:
		if spec.AppID == "" || spec.EnvironmentID == "" {
			msg = "a grant on an environment names the app and the environment"
		}
	default:
		return plxerr.New(plxerr.ScopeInvalid, "%q is not a scope; the scopes are organization, app and environment", spec.Scope)
	}
	if msg != "" {
		return plxerr.New(plxerr.ScopeInvalid, "%s", msg)
	}
	return nil
}

// holderExists checks that a grant's team exists in the organisation, or
// that its user belongs to it, directly or through a team.
func holderExists(ctx context.Context, q *dbgen.Queries, org, team, user pgtype.UUID) error {
	if team.Valid {
		if _, err := q.GetTeam(ctx, team); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return plxerr.New(plxerr.ResourceNotFound, "no such team")
			}
			return fmt.Errorf("auth: read a team: %w", err)
		}
		return nil
	}
	rows, err := q.ListMembershipsForUser(ctx, dbgen.ListMembershipsForUserParams{OrganizationID: org, UserID: user})
	if err != nil {
		return fmt.Errorf("auth: read memberships: %w", err)
	}
	if len(rows) == 0 {
		return plxerr.New(plxerr.ResourceNotFound, "no such member of the organisation")
	}
	return nil
}

// CreateGrant binds a role to a user or a team at a scope (GOV-003). It
// needs members.manage, the role must exist (PLX-8100), the scope must name
// an app or environment of the organisation (PLX-8102), and the role must
// hold nothing the caller does not hold.
func (s *Service) CreateGrant(ctx context.Context, p Principal, spec GrantSpec) (RoleGrantRecord, error) {
	if err := p.Authorize(MembersManage); err != nil {
		return RoleGrantRecord{}, err
	}
	if (spec.UserID == "") == (spec.TeamID == "") {
		return RoleGrantRecord{}, plxerr.New(plxerr.InvalidStructure, "a grant names exactly one of a team and a user")
	}
	user, err := storage.NullUUID(spec.UserID)
	if err != nil {
		return RoleGrantRecord{}, plxerr.New(plxerr.InvalidFormat, "the user identifier is not valid")
	}
	team, err := storage.NullUUID(spec.TeamID)
	if err != nil {
		return RoleGrantRecord{}, plxerr.New(plxerr.InvalidFormat, "the team identifier is not valid")
	}
	id, err := s.newID()
	if err != nil {
		return RoleGrantRecord{}, err
	}
	var out RoleGrantRecord
	err = s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		org := storage.MustUUID(p.OrganizationID)
		if err := roleHeld(ctx, q, p, spec.Role); err != nil {
			return err
		}
		app, env, err := checkScope(ctx, q, spec)
		if err != nil {
			return err
		}
		if err := holderExists(ctx, q, org, team, user); err != nil {
			return err
		}
		row, err := q.CreateRoleGrant(ctx, dbgen.CreateRoleGrantParams{
			ID: storage.MustUUID(id), OrganizationID: org, UserID: user, TeamID: team,
			Role: spec.Role, Scope: string(spec.Scope), AppID: app, EnvironmentID: env,
		})
		if err != nil {
			if uniqueViolation(err) {
				return plxerr.New(plxerr.ResourceExists, "the holder already has this role at this scope")
			}
			return fmt.Errorf("auth: create a grant: %w", err)
		}
		out = grantRecord(row)
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: p.OrganizationID, Actor: p.Actor(),
			Action: audit.RoleGranted, TargetKind: "grant", TargetID: out.ID, Detail: grantDetail(out),
		})
	})
	return out, err
}

// grantDetail says whom a grant is for, with which role and where.
func grantDetail(g RoleGrantRecord) string {
	holder := "user " + g.UserID
	if g.TeamID != "" {
		holder = "team " + g.TeamID
	}
	where := string(g.Scope)
	if g.AppID != "" {
		where += " app " + g.AppID
	}
	if g.EnvironmentID != "" {
		where += " environment " + g.EnvironmentID
	}
	return fmt.Sprintf("%s role %s on %s", holder, g.Role, where)
}

// UpdateGrant changes the role of a grant. Changing the owner role of the
// last owner away is PLX-8103.
func (s *Service) UpdateGrant(ctx context.Context, p Principal, grantID, role string) (RoleGrantRecord, error) {
	if err := p.Authorize(MembersManage); err != nil {
		return RoleGrantRecord{}, err
	}
	gid, err := parseID(grantID, "grant")
	if err != nil {
		return RoleGrantRecord{}, err
	}
	var out RoleGrantRecord
	err = s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := roleHeld(ctx, q, p, role); err != nil {
			return err
		}
		return GuardOwners(ctx, q, storage.MustUUID(p.OrganizationID), func() error {
			if _, err := q.GetRoleGrant(ctx, gid); err != nil {
				return notFound(err, "grant")
			}
			row, err := q.UpdateRoleGrant(ctx, dbgen.UpdateRoleGrantParams{ID: gid, Role: role})
			if err != nil {
				if uniqueViolation(err) {
					return plxerr.New(plxerr.ResourceExists, "the holder already has this role at this scope")
				}
				return fmt.Errorf("auth: update a grant: %w", err)
			}
			out = grantRecord(row)
			return s.record(ctx, tx, audit.Entry{
				OrganizationID: p.OrganizationID, Actor: p.Actor(),
				Action: audit.RoleGrantChanged, TargetKind: "grant", TargetID: grantID, Detail: grantDetail(out),
			})
		})
	})
	return out, err
}

// DeleteGrant removes a grant. Removing the last owner's is PLX-8103.
func (s *Service) DeleteGrant(ctx context.Context, p Principal, grantID string) error {
	if err := p.Authorize(MembersManage); err != nil {
		return err
	}
	gid, err := parseID(grantID, "grant")
	if err != nil {
		return err
	}
	return s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		return GuardOwners(ctx, q, storage.MustUUID(p.OrganizationID), func() error {
			row, err := q.GetRoleGrant(ctx, gid)
			if err != nil {
				return notFound(err, "grant")
			}
			if _, err := q.DeleteRoleGrant(ctx, gid); err != nil {
				return fmt.Errorf("auth: delete a grant: %w", err)
			}
			return s.record(ctx, tx, audit.Entry{
				OrganizationID: p.OrganizationID, Actor: p.Actor(),
				Action: audit.RoleRevoked, TargetKind: "grant", TargetID: grantID, Detail: grantDetail(grantRecord(row)),
			})
		})
	})
}

// GuardOwners runs a change and refuses it with PLX-8103 when it leaves an
// organisation that had an owner without one (GOV-002). The change runs in
// the caller's transaction, so a refusal rolls it back. Owners are people
// with the owner role by a direct membership or by a grant on the
// organisation, to them or to a team they belong to.
func GuardOwners(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, change func() error) error {
	before, err := q.CountOwners(ctx, org)
	if err != nil {
		return fmt.Errorf("auth: count owners: %w", err)
	}
	if err := change(); err != nil {
		return err
	}
	if before == 0 {
		return nil
	}
	after, err := q.CountOwners(ctx, org)
	if err != nil {
		return fmt.Errorf("auth: count owners: %w", err)
	}
	if after == 0 {
		return plxerr.New(plxerr.LastOwnerCannotBeRemoved, "an organisation keeps at least one owner; make someone else owner first")
	}
	return nil
}
