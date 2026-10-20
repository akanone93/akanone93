// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/identity"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// This file is the core's side of SCIM provisioning (GOV-004, ADR-0074):
// the identity.Directory the enterprise SCIM server drives. It owns every
// decision, so a SCIM client can do nothing the core's rules refuse. A SCIM
// user is an account of the installation with a record for the
// organisation; a SCIM group is a team. Deprovisioning removes the person's
// access to the organisation and their tokens there at once and, when they
// belong nowhere else, disables the account and ends their sessions.

// scimActor is how the audit log names the organisation's SCIM client.
var scimActor = audit.Actor{Kind: "scim", Display: "SCIM client"}

// scimDirectory implements identity.Directory over the service.
type scimDirectory struct{ s *Service }

var _ identity.Directory = scimDirectory{}

// SCIMDirectory returns the directory the SCIM server works on.
func (s *Service) SCIMDirectory() identity.Directory { return scimDirectory{s: s} }

// Authenticate resolves a SCIM bearer token to its organisation.
func (d scimDirectory) Authenticate(ctx context.Context, token string) (string, error) {
	return d.s.authenticateScim(ctx, token)
}

// Limits reports the largest page a client may ask for.
func (d scimDirectory) Limits() identity.DirectoryLimits {
	return identity.DirectoryLimits{MaxResults: int(d.s.o.Limits.Get(limits.APIPageSize))}
}

// invalidValue is an ErrInvalid with a reason.
func invalidValue(format string, args ...any) error {
	return fmt.Errorf("%w: %s", identity.ErrInvalid, fmt.Sprintf(format, args...))
}

// refused is an ErrRefused with a reason.
func refused(format string, args ...any) error {
	return fmt.Errorf("%w: %s", identity.ErrRefused, fmt.Sprintf(format, args...))
}

// resource parses the identifier of a resource; one that is not an
// identifier names nothing.
func resource(id string) (pgtype.UUID, error) {
	u, err := storage.UUID(id)
	if err != nil {
		return pgtype.UUID{}, identity.ErrNotFound
	}
	return u, nil
}

// scimWrite refuses a write the licence does not allow. The SCIM server
// checks too; the directory is the last gate.
func scimWrite(ctx context.Context) error {
	return edition.Require(ctx, edition.FeatureSCIM) //nolint:wrapcheck // a domain error
}

// scimTenant is what a transaction sees: the organisation, and the person's
// own grants everywhere, to tell whether they belong anywhere else.
func scimTenant(org, user string) storage.Tenant {
	return storage.Tenant{OrganizationID: org, UserID: user}
}

// scimRecord is a SCIM user's record joined to the account.
type scimRecord struct {
	userID          pgtype.UUID
	externalID      string
	displayName     string
	active          bool
	createdAccount  bool
	disabledAccount bool
	version         int64
	created         time.Time
	updated         time.Time
	email           string
	accountName     string
}

// userView builds the SCIM user of a record, with its groups.
func userView(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, r scimRecord) (identity.User, error) {
	refs, err := q.ListScimGroupRefsOfUser(ctx, dbgen.ListScimGroupRefsOfUserParams{OrganizationID: org, UserID: r.userID})
	if err != nil {
		return identity.User{}, fmt.Errorf("auth: read a user's groups: %w", err)
	}
	name := r.displayName
	if name == "" {
		name = r.accountName
	}
	u := identity.User{
		ID: storage.ID(r.userID), ExternalID: r.externalID, UserName: r.email, DisplayName: name, Active: r.active,
		Version: r.version, Created: r.created, Modified: r.updated,
	}
	for _, ref := range refs {
		u.Groups = append(u.Groups, identity.GroupRef{ID: storage.ID(ref.ID), Display: ref.Name})
	}
	return u, nil
}

// recordOf converts the joined row of one user.
func recordOf(r dbgen.GetScimUserWithAccountRow) scimRecord {
	return scimRecord{
		userID: r.UserID, externalID: r.ExternalID, displayName: r.DisplayName, active: r.Active,
		createdAccount: r.CreatedAccount, disabledAccount: r.DisabledAccount, version: r.Version,
		created: storage.Time(r.CreatedAt), updated: storage.Time(r.UpdatedAt), email: r.Email, accountName: r.AccountName,
	}
}

// loadUser reads a user's record and view.
func loadUser(ctx context.Context, q *dbgen.Queries, org, user pgtype.UUID) (identity.User, error) {
	row, err := q.GetScimUserWithAccount(ctx, dbgen.GetScimUserWithAccountParams{OrganizationID: org, UserID: user})
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.User{}, identity.ErrNotFound
	}
	if err != nil {
		return identity.User{}, fmt.Errorf("auth: read a SCIM user: %w", err)
	}
	return userView(ctx, q, org, recordOf(row))
}

// GetUser returns one user.
func (d scimDirectory) GetUser(ctx context.Context, org, id string) (identity.User, error) {
	user, err := resource(id)
	if err != nil {
		return identity.User{}, err
	}
	var out identity.User
	err = d.s.inTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = loadUser(ctx, dbgen.New(tx), storage.MustUUID(org), user)
		return err
	})
	return out, err
}

// page clamps a query to the directory's limits and returns the page size
// and offset.
func (d scimDirectory) page(q identity.ListQuery) (size, offset int32) {
	ceiling := int(d.s.o.Limits.Get(limits.APIPageSize))
	count := q.Count
	if count <= 0 || count > ceiling {
		count = ceiling
	}
	start := min(max(q.StartIndex, 1), 1<<30)
	return int32(count), int32(start - 1) //nolint:gosec // bounded by the page size limit and 1<<30
}

// checkFilter refuses a filter on an attribute the directory cannot search.
func checkFilter(attribute string, allowed ...string) error {
	if attribute == "" {
		return nil
	}
	for _, a := range allowed {
		if a == attribute {
			return nil
		}
	}
	return invalidValue("the attribute %q cannot be filtered", attribute)
}

// ListUsers lists a page of users, optionally those with one attribute equal
// to a value.
func (d scimDirectory) ListUsers(ctx context.Context, org string, q identity.ListQuery) (identity.UserPage, error) {
	if err := checkFilter(q.Attribute, "userName", "externalId", "displayName"); err != nil {
		return identity.UserPage{}, err
	}
	size, offset := d.page(q)
	var out identity.UserPage
	err := d.s.inTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		queries, orgID := dbgen.New(tx), storage.MustUUID(org)
		rows, err := queries.ListScimUsers(ctx, dbgen.ListScimUsersParams{
			OrganizationID: orgID, Attribute: q.Attribute, Value: q.Value, PageSize: size, PageOffset: offset,
		})
		if err != nil {
			return fmt.Errorf("auth: list SCIM users: %w", err)
		}
		for _, r := range rows {
			u, err := userView(ctx, queries, orgID, scimRecord{
				userID: r.UserID, externalID: r.ExternalID, displayName: r.DisplayName, active: r.Active,
				version: r.Version, created: storage.Time(r.CreatedAt), updated: storage.Time(r.UpdatedAt),
				email: r.Email, accountName: r.AccountName,
			})
			if err != nil {
				return err
			}
			out.Users = append(out.Users, u)
			out.Total = int(r.Total)
		}
		if len(rows) == 0 {
			n, err := queries.CountScimUsers(ctx, dbgen.CountScimUsersParams{OrganizationID: orgID, Attribute: q.Attribute, Value: q.Value})
			if err != nil {
				return fmt.Errorf("auth: count SCIM users: %w", err)
			}
			out.Total = int(n)
		}
		return nil
	})
	return out, err
}

// CreateUser provisions a person, or adopts the account that already has
// their address.
func (d scimDirectory) CreateUser(ctx context.Context, org string, spec identity.UserSpec) (identity.User, error) {
	if err := scimWrite(ctx); err != nil {
		return identity.User{}, err
	}
	email, err := normaliseEmail(spec.UserName)
	if err != nil {
		return identity.User{}, invalidValue("userName must be an email address")
	}
	var out identity.User
	err = d.s.inTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		q, orgID := dbgen.New(tx), storage.MustUUID(org)
		account, created, err := d.s.scimAccount(ctx, tx, email, spec.DisplayName)
		if err != nil {
			return err
		}
		if err := d.s.scimRecordFor(ctx, q, orgID, account, created, spec); err != nil {
			return err
		}
		if out, err = loadUser(ctx, q, orgID, account.ID); err != nil {
			return err
		}
		return d.s.record(ctx, tx, audit.Entry{
			OrganizationID: org, Actor: scimActor, Action: audit.ScimUserProvisioned,
			TargetKind: "user", TargetID: out.ID, Detail: fmt.Sprintf("active=%t", spec.Active),
		})
	})
	return out, err
}

// scimAccount returns the account with an address, creating one that has no
// password and no invitation when there is none: its only way in is the
// organisation's identity provider.
func (s *Service) scimAccount(ctx context.Context, tx pgx.Tx, email, display string) (dbgen.User, bool, error) {
	q := dbgen.New(tx)
	row, err := q.GetUserByEmail(ctx, email)
	if err == nil {
		return row, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return dbgen.User{}, false, fmt.Errorf("auth: read a user: %w", err)
	}
	id, err := s.newID()
	if err != nil {
		return dbgen.User{}, false, err
	}
	if strings.TrimSpace(display) == "" {
		display, _, _ = strings.Cut(email, "@")
	}
	row, err = q.CreateUser(ctx, dbgen.CreateUserParams{ID: storage.MustUUID(id), Email: email, DisplayName: strings.TrimSpace(display)})
	if err != nil {
		return dbgen.User{}, false, fmt.Errorf("auth: create a user: %w", err)
	}
	return row, true, nil
}

// scimRecordFor makes the organisation's SCIM record of an account: new, or
// the revived record of a person deleted before. A record that is live
// already is a conflict.
func (s *Service) scimRecordFor(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, account dbgen.User, created bool, spec identity.UserSpec) error {
	rec, err := q.GetScimUserForUpdate(ctx, dbgen.GetScimUserForUpdateParams{OrganizationID: org, UserID: account.ID})
	switch {
	case err == nil && !rec.DeletedAt.Valid:
		return identity.ErrConflict
	case err == nil:
		disabled := rec.DisabledAccount
		if disabled && spec.Active {
			if _, err := q.EnableUser(ctx, account.ID); err != nil {
				return fmt.Errorf("auth: enable a user: %w", err)
			}
			disabled = false
		}
		_, err := q.UpdateScimUser(ctx, dbgen.UpdateScimUserParams{
			OrganizationID: org, UserID: account.ID, ExternalID: spec.ExternalID, DisplayName: spec.DisplayName,
			Active: spec.Active, DisabledAccount: disabled,
		})
		return scimWriteError(err, "revive a SCIM user")
	case !errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("auth: read a SCIM user: %w", err)
	}
	if account.DisabledAt.Valid && spec.Active {
		return refused("the account is disabled")
	}
	_, err = q.CreateScimUser(ctx, dbgen.CreateScimUserParams{
		OrganizationID: org, UserID: account.ID, ExternalID: spec.ExternalID, DisplayName: spec.DisplayName,
		Active: spec.Active, CreatedAccount: created,
	})
	return scimWriteError(err, "create a SCIM user")
}

// scimWriteError turns a storage failure into the directory's errors.
func scimWriteError(err error, what string) error {
	switch {
	case err == nil:
		return nil
	case uniqueViolation(err):
		return identity.ErrConflict
	default:
		return fmt.Errorf("auth: %s: %w", what, err)
	}
}

// ReplaceUser sets a user to spec: name, external identifier and whether
// they are active. Setting a person inactive deprovisions them.
func (d scimDirectory) ReplaceUser(ctx context.Context, org, id string, spec identity.UserSpec, ifVersion int64) (identity.User, error) {
	if err := scimWrite(ctx); err != nil {
		return identity.User{}, err
	}
	user, err := resource(id)
	if err != nil {
		return identity.User{}, err
	}
	var out identity.User
	err = d.s.inTx(ctx, scimTenant(org, id), func(ctx context.Context, tx pgx.Tx) error {
		q, orgID := dbgen.New(tx), storage.MustUUID(org)
		rec, err := q.GetScimUserForUpdate(ctx, dbgen.GetScimUserForUpdateParams{OrganizationID: orgID, UserID: user})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && rec.DeletedAt.Valid) {
			return identity.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("auth: read a SCIM user: %w", err)
		}
		if ifVersion != 0 && ifVersion != rec.Version {
			return identity.ErrVersionMismatch
		}
		disabled, action, err := d.s.applyUserSpec(ctx, tx, orgID, rec, spec)
		if err != nil {
			return err
		}
		if _, err := q.UpdateScimUser(ctx, dbgen.UpdateScimUserParams{
			OrganizationID: orgID, UserID: user, ExternalID: spec.ExternalID, DisplayName: spec.DisplayName,
			Active: spec.Active, DisabledAccount: disabled,
		}); err != nil {
			return scimWriteError(err, "update a SCIM user")
		}
		if out, err = loadUser(ctx, q, orgID, user); err != nil {
			return err
		}
		return d.s.record(ctx, tx, audit.Entry{
			OrganizationID: org, Actor: scimActor, Action: action, TargetKind: "user", TargetID: id,
			Detail: fmt.Sprintf("active=%t", spec.Active),
		})
	})
	return out, err
}

// applyUserSpec applies what a replace changes besides the SCIM record: the
// account's address and name where SCIM made the account, and the
// deprovisioning or reactivation an active flag asks for. It returns
// whether the account is now disabled by SCIM and the action to audit.
func (s *Service) applyUserSpec(ctx context.Context, tx pgx.Tx, org pgtype.UUID, rec dbgen.ScimUser, spec identity.UserSpec) (bool, audit.Action, error) {
	q := dbgen.New(tx)
	account, err := q.GetUser(ctx, rec.UserID)
	if err != nil {
		return false, "", fmt.Errorf("auth: read a user: %w", err)
	}
	email, err := normaliseEmail(spec.UserName)
	if err != nil {
		return false, "", invalidValue("userName must be an email address")
	}
	if !strings.EqualFold(email, account.Email) {
		if !rec.CreatedAccount {
			return false, "", invalidValue("userName cannot be changed: the account was not created by SCIM")
		}
		if _, err := q.SetUserEmail(ctx, dbgen.SetUserEmailParams{ID: account.ID, Email: email}); err != nil {
			return false, "", scimWriteError(err, "change a user's address")
		}
	}
	if rec.CreatedAccount && strings.TrimSpace(spec.DisplayName) != "" {
		if err := q.SetUserDisplayName(ctx, dbgen.SetUserDisplayNameParams{ID: account.ID, DisplayName: strings.TrimSpace(spec.DisplayName)}); err != nil {
			return false, "", fmt.Errorf("auth: set a display name: %w", err)
		}
	}
	disabled := rec.DisabledAccount
	switch {
	case rec.Active && !spec.Active:
		disabled, err = s.deprovision(ctx, tx, org, account)
		return disabled, audit.ScimUserDeprovisioned, err
	case !rec.Active && spec.Active && rec.DisabledAccount:
		if _, err := q.EnableUser(ctx, account.ID); err != nil {
			return false, "", fmt.Errorf("auth: enable a user: %w", err)
		}
		disabled = false
	}
	return disabled, audit.ScimUserChanged, nil
}

// deprovision removes a person's access to an organisation and revokes the
// tokens they hold there, at once. When they have access nowhere else the
// account is disabled and all their sessions and tokens end. An installation
// administrator, or the last owner of the organisation, is not deprovisioned
// by an organisation's client.
func (s *Service) deprovision(ctx context.Context, tx pgx.Tx, org pgtype.UUID, account dbgen.User) (bool, error) {
	if account.InstallationAdmin {
		return false, refused("an installation administrator cannot be deprovisioned by an organisation")
	}
	q := dbgen.New(tx)
	err := GuardOwners(ctx, q, org, func() error {
		if _, err := q.DeleteMembershipsOfUserIn(ctx, dbgen.DeleteMembershipsOfUserInParams{OrganizationID: org, UserID: account.ID}); err != nil {
			return fmt.Errorf("auth: remove memberships: %w", err)
		}
		if _, err := q.DeleteGrantsOfUserIn(ctx, dbgen.DeleteGrantsOfUserInParams{OrganizationID: org, UserID: account.ID}); err != nil {
			return fmt.Errorf("auth: remove grants: %w", err)
		}
		if _, err := q.DeleteAppAccessOfUserIn(ctx, dbgen.DeleteAppAccessOfUserInParams{OrganizationID: org, UserID: account.ID}); err != nil {
			return fmt.Errorf("auth: remove app access: %w", err)
		}
		return nil
	})
	if errors.Is(err, plxerr.Sentinel(plxerr.LastOwnerCannotBeRemoved)) {
		return false, refused("the person is the last owner of the organisation")
	}
	if err != nil {
		return false, err
	}
	if _, err := q.RevokeUserTokensIn(ctx, dbgen.RevokeUserTokensInParams{OrganizationID: org, UserID: account.ID}); err != nil {
		return false, fmt.Errorf("auth: revoke tokens: %w", err)
	}
	left, err := q.CountOrganizationAccess(ctx, account.ID)
	if err != nil {
		return false, fmt.Errorf("auth: count access: %w", err)
	}
	if left != 0 {
		return false, nil
	}
	if _, err := q.DisableUser(ctx, account.ID); err != nil {
		return false, fmt.Errorf("auth: disable a user: %w", err)
	}
	if _, err := q.RevokeAllUserSessions(ctx, account.ID); err != nil {
		return false, fmt.Errorf("auth: end sessions: %w", err)
	}
	if _, err := q.RevokeUserTokens(ctx, account.ID); err != nil {
		return false, fmt.Errorf("auth: revoke tokens: %w", err)
	}
	return true, nil
}

// DeleteUser deprovisions a person and removes them from SCIM's view.
func (d scimDirectory) DeleteUser(ctx context.Context, org, id string) error {
	if err := scimWrite(ctx); err != nil {
		return err
	}
	user, err := resource(id)
	if err != nil {
		return err
	}
	return d.s.inTx(ctx, scimTenant(org, id), func(ctx context.Context, tx pgx.Tx) error {
		q, orgID := dbgen.New(tx), storage.MustUUID(org)
		rec, err := q.GetScimUserForUpdate(ctx, dbgen.GetScimUserForUpdateParams{OrganizationID: orgID, UserID: user})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && rec.DeletedAt.Valid) {
			return identity.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("auth: read a SCIM user: %w", err)
		}
		account, err := q.GetUser(ctx, user)
		if err != nil {
			return fmt.Errorf("auth: read a user: %w", err)
		}
		disabled := rec.DisabledAccount
		if rec.Active {
			if disabled, err = d.s.deprovision(ctx, tx, orgID, account); err != nil {
				return err
			}
			if _, err := q.UpdateScimUser(ctx, dbgen.UpdateScimUserParams{
				OrganizationID: orgID, UserID: user, ExternalID: rec.ExternalID, DisplayName: rec.DisplayName,
				Active: false, DisabledAccount: disabled,
			}); err != nil {
				return scimWriteError(err, "update a SCIM user")
			}
		}
		if _, err := q.DeleteScimUser(ctx, dbgen.DeleteScimUserParams{OrganizationID: orgID, UserID: user}); err != nil {
			return fmt.Errorf("auth: delete a SCIM user: %w", err)
		}
		return d.s.record(ctx, tx, audit.Entry{
			OrganizationID: org, Actor: scimActor, Action: audit.ScimUserDeprovisioned, TargetKind: "user", TargetID: id, Detail: "deleted",
		})
	})
}
