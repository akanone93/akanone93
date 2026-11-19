// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package tenancy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Organization is a tenant (GOV-001).
type Organization struct {
	ID        string
	Key       string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Team groups members of an organisation.
type Team struct {
	ID             string
	OrganizationID string
	Key            string
	Name           string
	CreatedAt      time.Time
}

// Member binds a user to an organisation, or to one of its teams.
type Member struct {
	// MembershipID identifies the membership itself.
	MembershipID   string
	UserID         string
	OrganizationID string
	TeamID         string
	Role           string
	AddedAt        time.Time
	DisplayName    string
	Email          string
}

// teamRole is the role of every team membership: a team's members hold
// the roles the team is granted on apps (GOV-001).
const teamRole = "member"

// CreateOrganization creates an organisation and makes its creator the
// owner. Only an installation administrator may create one.
func (s *Service) CreateOrganization(ctx context.Context, id auth.Identity, key, name string) (Organization, error) {
	if id.Kind != auth.KindUser || !id.InstallationAdmin {
		return Organization{}, plxerr.New(plxerr.PermissionDenied, "only an installation administrator may create an organisation")
	}
	name = strings.TrimSpace(name)
	if err := checkKey("organisation", key); err != nil {
		return Organization{}, err
	}
	if err := checkName("organisation", name); err != nil {
		return Organization{}, err
	}
	orgID, err := s.newID()
	if err != nil {
		return Organization{}, err
	}
	membershipID, err := s.newID()
	if err != nil {
		return Organization{}, err
	}
	owner := auth.Principal{Identity: id, OrganizationID: orgID}
	var out Organization
	err = s.inOrg(ctx, owner, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
			ID: storage.MustUUID(orgID), Key: key, Name: name,
		})
		if err != nil {
			return failure(err, "organisation")
		}
		if _, err := q.AddMembership(ctx, dbgen.AddMembershipParams{
			ID: storage.MustUUID(membershipID), OrganizationID: row.ID,
			UserID: storage.MustUUID(id.UserID), Role: string(auth.RoleOwner),
		}); err != nil {
			return failure(err, "membership")
		}
		out = organizationOf(row)
		return s.record(ctx, tx, owner, audit.OrganizationCreated, "organization", orgID)
	})
	return out, err
}

// GetOrganization returns the principal's organisation.
func (s *Service) GetOrganization(ctx context.Context, p auth.Principal) (Organization, error) {
	if err := authorize(p, auth.OrganizationRead); err != nil {
		return Organization{}, err
	}
	var out Organization
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetOrganization(ctx, storage.MustUUID(p.OrganizationID))
		if err != nil {
			return failure(err, "organisation")
		}
		out = organizationOf(row)
		return nil
	})
	return out, err
}

// ListOrganizations lists the organisations a person belongs to, in key
// order.
func (s *Service) ListOrganizations(ctx context.Context, id auth.Identity, page Page) ([]Organization, error) {
	if id.UserID == "" {
		return nil, plxerr.New(plxerr.PreconditionFailed, "this credential belongs to no person")
	}
	var out []Organization
	err := s.o.DB.InTx(ctx, storage.Tenant{UserID: id.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListOrganizationsForUser(ctx, dbgen.ListOrganizationsForUserParams{
			UserID: storage.MustUUID(id.UserID), AfterKey: page.After.Key, AfterID: page.After.AfterID(), PageSize: page.Size,
		})
		if err != nil {
			return failure(err, "organisation")
		}
		for _, row := range rows {
			// A token sees only the organisation it is bound to.
			if id.OrganizationID != "" && storage.ID(row.ID) != id.OrganizationID {
				continue
			}
			out = append(out, organizationOf(row))
		}
		return nil
	})
	return out, err //nolint:wrapcheck // InTx wraps its own failures
}

// UpdateOrganization renames the principal's organisation.
func (s *Service) UpdateOrganization(ctx context.Context, p auth.Principal, name string) (Organization, error) {
	if err := authorize(p, auth.OrganizationManage); err != nil {
		return Organization{}, err
	}
	name = strings.TrimSpace(name)
	if err := checkName("organisation", name); err != nil {
		return Organization{}, err
	}
	var out Organization
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).UpdateOrganization(ctx, dbgen.UpdateOrganizationParams{
			ID: storage.MustUUID(p.OrganizationID), Name: name,
		})
		if err != nil {
			return failure(err, "organisation")
		}
		out = organizationOf(row)
		return s.record(ctx, tx, p, audit.OrganizationUpdated, "organization", p.OrganizationID)
	})
	return out, err
}

// CreateTeam adds a team to the principal's organisation.
func (s *Service) CreateTeam(ctx context.Context, p auth.Principal, key, name string) (Team, error) {
	if err := authorize(p, auth.TeamsManage); err != nil {
		return Team{}, err
	}
	name = strings.TrimSpace(name)
	if err := checkKey("team", key); err != nil {
		return Team{}, err
	}
	if err := checkName("team", name); err != nil {
		return Team{}, err
	}
	id, err := s.newID()
	if err != nil {
		return Team{}, err
	}
	var out Team
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).CreateTeam(ctx, dbgen.CreateTeamParams{
			ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(p.OrganizationID), Key: key, Name: name,
		})
		if err != nil {
			return failure(err, "team")
		}
		out = teamOf(row)
		return s.record(ctx, tx, p, audit.TeamCreated, "team", id)
	})
	return out, err
}

// ListTeams lists the principal's organisation's teams, in key order.
func (s *Service) ListTeams(ctx context.Context, p auth.Principal, page Page) ([]Team, error) {
	if err := authorize(p, auth.OrganizationRead); err != nil {
		return nil, err
	}
	var out []Team
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListTeams(ctx, dbgen.ListTeamsParams{
			OrganizationID: storage.MustUUID(p.OrganizationID),
			AfterKey:       page.After.Key, AfterID: page.After.AfterID(), PageSize: page.Size,
		})
		if err != nil {
			return failure(err, "team")
		}
		out = make([]Team, len(rows))
		for i, row := range rows {
			out[i] = teamOf(row)
		}
		return nil
	})
	return out, err
}

// UpdateTeam renames a team.
func (s *Service) UpdateTeam(ctx context.Context, p auth.Principal, teamID, name string) (Team, error) {
	if err := authorize(p, auth.TeamsManage); err != nil {
		return Team{}, err
	}
	tid, err := parseID(teamID, "team")
	if err != nil {
		return Team{}, err
	}
	name = strings.TrimSpace(name)
	if err := checkName("team", name); err != nil {
		return Team{}, err
	}
	var out Team
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).UpdateTeam(ctx, dbgen.UpdateTeamParams{ID: tid, Name: name})
		if err != nil {
			return failure(err, "team")
		}
		out = teamOf(row)
		return s.record(ctx, tx, p, audit.TeamUpdated, "team", teamID)
	})
	return out, err
}

// DeleteTeam removes a team with its memberships and app grants.
func (s *Service) DeleteTeam(ctx context.Context, p auth.Principal, teamID string) error {
	if err := authorize(p, auth.TeamsManage); err != nil {
		return err
	}
	tid, err := parseID(teamID, "team")
	if err != nil {
		return err
	}
	return s.inOrgChanging(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		// A team can hold the owner role, so deleting it can remove the
		// last owner (PLX-8103).
		return auth.GuardOwners(ctx, q, storage.MustUUID(p.OrganizationID), func() error {
			n, err := q.DeleteTeam(ctx, tid)
			if err != nil {
				return failure(err, "team")
			}
			if n == 0 {
				return plxerr.New(plxerr.ResourceNotFound, "no such team")
			}
			return s.record(ctx, tx, p, audit.TeamDeleted, "team", teamID)
		})
	})
}

// AddMember grants a role in the organisation, or membership of one of
// its teams, to the account with an email address. When there is no
// such account it is created with a one-time invitation, which is
// returned exactly once for the administrator to pass on.
func (s *Service) AddMember(ctx context.Context, p auth.Principal, teamID, email, role string) (Member, string, error) {
	if err := authorize(p, auth.MembersManage); err != nil {
		return Member{}, "", err
	}
	team, role, err := membershipRole(teamID, role)
	if err != nil {
		return Member{}, "", err
	}
	membershipID, err := s.newID()
	if err != nil {
		return Member{}, "", err
	}
	// An account that has not accepted an earlier invitation gets a fresh
	// one, which voids the earlier, when it is invited again after it was
	// removed or when its invitation has expired — but only when no other
	// organisation holds a membership of it: otherwise this organisation
	// could take over an account another one invited (build log #169).
	renew, err := s.mayRenewInvitation(ctx, p, email)
	if err != nil {
		return Member{}, "", err
	}
	var (
		out        Member
		invitation string
	)
	err = s.inOrgChanging(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		user, inv, err := s.o.Auth.Invite(ctx, tx, email)
		if err != nil {
			return err //nolint:wrapcheck // already a domain error
		}
		if inv == "" && renew != "" && renew == user.ID {
			if inv, err = s.o.Auth.RenewInvitation(ctx, tx, user.ID); err != nil {
				return err //nolint:wrapcheck // already wrapped by the auth service
			}
		}
		invitation = inv
		if inv != "" {
			if err := s.record(ctx, tx, p, audit.UserInvited, "user", user.ID); err != nil {
				return err
			}
		}
		row, err := addMembership(ctx, q, p.OrganizationID, membershipID, user.ID, team, role)
		if err != nil {
			return err
		}
		out = memberOf(row, user.DisplayName, user.Email)
		return s.record(ctx, tx, p, audit.MemberAdded, "user", user.ID)
	})
	return out, invitation, err
}

// membershipRole checks the role of a new membership: one of the roles
// for the organisation itself, and "member" for a team.
func membershipRole(teamID, role string) (pgtype.UUID, string, error) {
	if teamID == "" {
		return pgtype.UUID{}, role, checkRole(role)
	}
	team, err := parseID(teamID, "team")
	if err != nil {
		return pgtype.UUID{}, "", err
	}
	if role != "" && role != teamRole {
		return pgtype.UUID{}, "", plxerr.New(plxerr.InvalidEnumValue,
			"a team membership's role is %q: a team's members hold the roles the team is granted on apps", teamRole)
	}
	return team, teamRole, nil
}

// addMembership writes a membership of the organisation or of a team.
func addMembership(ctx context.Context, q *dbgen.Queries, org, membershipID, userID string, team pgtype.UUID, role string) (dbgen.Membership, error) {
	var (
		row dbgen.Membership
		err error
	)
	if team.Valid {
		if _, err := q.GetTeam(ctx, team); err != nil {
			return dbgen.Membership{}, failure(err, "team")
		}
		row, err = q.AddTeamMembership(ctx, dbgen.AddTeamMembershipParams{
			ID: storage.MustUUID(membershipID), OrganizationID: storage.MustUUID(org),
			UserID: storage.MustUUID(userID), TeamID: team, Role: role,
		})
	} else {
		// Setting a role can demote the last owner (PLX-8103).
		err = auth.GuardOwners(ctx, q, storage.MustUUID(org), func() error {
			var addErr error
			row, addErr = q.AddMembership(ctx, dbgen.AddMembershipParams{
				ID: storage.MustUUID(membershipID), OrganizationID: storage.MustUUID(org),
				UserID: storage.MustUUID(userID), Role: role,
			})
			if addErr != nil {
				return fmt.Errorf("tenancy: add a membership: %w", addErr)
			}
			return nil
		})
	}
	if err != nil {
		return dbgen.Membership{}, failure(err, "membership")
	}
	return row, nil
}

// RemoveMember removes a membership of the organisation or of a team.
func (s *Service) RemoveMember(ctx context.Context, p auth.Principal, teamID, userID string) error {
	if err := authorize(p, auth.MembersManage); err != nil {
		return err
	}
	uid, err := parseID(userID, "user")
	if err != nil {
		return err
	}
	var team pgtype.UUID
	if teamID != "" {
		if team, err = parseID(teamID, "team"); err != nil {
			return err
		}
	}
	// A pending account's invitation is voided with its last membership,
	// unless another organisation also holds one (build log #169).
	elsewhere, err := s.memberElsewhere(ctx, p.OrganizationID, userID)
	if err != nil {
		return err
	}
	return s.inOrgChanging(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		// Removing a person can remove the last owner (PLX-8103), directly
		// or through a team that holds the owner role.
		return auth.GuardOwners(ctx, q, storage.MustUUID(p.OrganizationID), func() error {
			n, err := q.RemoveMembership(ctx, dbgen.RemoveMembershipParams{
				OrganizationID: storage.MustUUID(p.OrganizationID), UserID: uid, TeamID: team,
			})
			if err != nil {
				return failure(err, "membership")
			}
			if n == 0 {
				return plxerr.New(plxerr.ResourceNotFound, "no such membership")
			}
			if err := s.record(ctx, tx, p, audit.MemberRemoved, "user", userID); err != nil {
				return err
			}
			return s.voidLastInvitation(ctx, tx, p, userID, elsewhere)
		})
	})
}

// voidLastInvitation voids the invitation of an account that has not
// accepted it once it holds no membership of this organisation and none
// elsewhere, so a removed person's invitation cannot be used later, for
// example after they are invited again (build log #169).
func (s *Service) voidLastInvitation(ctx context.Context, tx pgx.Tx, p auth.Principal, userID string, elsewhere bool) error {
	if elsewhere || s.o.Auth == nil {
		return nil
	}
	left, err := dbgen.New(tx).CountMembershipsIn(ctx, dbgen.CountMembershipsInParams{
		OrganizationID: storage.MustUUID(p.OrganizationID), UserID: storage.MustUUID(userID),
	})
	if err != nil {
		return failure(err, "membership")
	}
	if left > 0 {
		return nil
	}
	revoked, err := s.o.Auth.RevokeInvitation(ctx, tx, userID)
	if err != nil || !revoked {
		return err //nolint:wrapcheck // already wrapped by the auth service
	}
	return s.record(ctx, tx, p, audit.InvitationRevoked, "user", userID)
}

// memberElsewhere reports whether the user holds a membership of an
// organisation other than orgID. It reads in the 'invitation' scope, which
// admits counting memberships across organisations for this purpose only;
// the answer never leaves the service (build log #169).
func (s *Service) memberElsewhere(ctx context.Context, orgID, userID string) (bool, error) {
	uid, err := parseID(userID, "user")
	if err != nil {
		return false, err
	}
	var n int64
	err = s.o.DB.InTx(ctx, storage.Tenant{Scope: storage.ScopeInvitation}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		n, err = dbgen.New(tx).CountMembershipsElsewhere(ctx, dbgen.CountMembershipsElsewhereParams{
			UserID: uid, OrganizationID: storage.MustUUID(orgID),
		})
		if err != nil {
			return failure(err, "membership")
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("tenancy: count memberships: %w", err)
	}
	return n > 0, nil
}

// mayRenewInvitation returns the ID of the pending account with this email
// address when a new invitation may be issued to it, and "" otherwise. It
// may when no other organisation holds a membership of the account, and
// the account either holds none here (it is being invited again) or its
// invitation has expired; a person who is still a member keeps the
// invitation they were given.
func (s *Service) mayRenewInvitation(ctx context.Context, p auth.Principal, email string) (string, error) {
	if s.o.Auth == nil {
		return "", nil
	}
	var pending string
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		id, live, err := s.o.Auth.PendingAccount(ctx, tx, email)
		if err != nil || id == "" {
			return err //nolint:wrapcheck // already a domain error
		}
		here, err := dbgen.New(tx).CountMembershipsIn(ctx, dbgen.CountMembershipsInParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), UserID: storage.MustUUID(id),
		})
		if err != nil {
			return failure(err, "membership")
		}
		if here == 0 || !live {
			pending = id
		}
		return nil
	})
	if err != nil || pending == "" {
		return "", err
	}
	elsewhere, err := s.memberElsewhere(ctx, p.OrganizationID, pending)
	if err != nil || elsewhere {
		return "", err
	}
	return pending, nil
}

// ListMembers lists the direct members of the organisation, or the
// members of one team, ordered by user. The cursor's ID is the user and
// its Key the membership, which breaks ties.
func (s *Service) ListMembers(ctx context.Context, p auth.Principal, teamID string, page Page) ([]Member, error) {
	if err := authorize(p, auth.OrganizationRead); err != nil {
		return nil, err
	}
	var team pgtype.UUID
	if teamID != "" {
		var err error
		if team, err = parseID(teamID, "team"); err != nil {
			return nil, err
		}
	}
	var out []Member
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListMembers(ctx, dbgen.ListMembersParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), TeamID: team,
			AfterUser: page.After.AfterID(), AfterID: storage.Cursor{ID: page.After.Key}.AfterID(), PageSize: page.Size,
		})
		if err != nil {
			return failure(err, "membership")
		}
		out = make([]Member, len(rows))
		for i, row := range rows {
			out[i] = Member{
				MembershipID: storage.ID(row.ID), UserID: storage.ID(row.UserID), OrganizationID: storage.ID(row.OrganizationID),
				TeamID: storage.ID(row.TeamID), Role: row.Role, AddedAt: storage.Time(row.AddedAt),
				DisplayName: row.DisplayName, Email: row.Email,
			}
		}
		return nil
	})
	return out, err
}

// organizationOf converts a stored organisation.
func organizationOf(row dbgen.Organization) Organization {
	return Organization{
		ID: storage.ID(row.ID), Key: row.Key, Name: row.Name,
		CreatedAt: storage.Time(row.CreatedAt), UpdatedAt: storage.Time(row.UpdatedAt),
	}
}

// teamOf converts a stored team.
func teamOf(row dbgen.Team) Team {
	return Team{
		ID: storage.ID(row.ID), OrganizationID: storage.ID(row.OrganizationID),
		Key: row.Key, Name: row.Name, CreatedAt: storage.Time(row.CreatedAt),
	}
}

// memberOf converts a stored membership.
func memberOf(row dbgen.Membership, display, email string) Member {
	return Member{
		MembershipID: storage.ID(row.ID), UserID: storage.ID(row.UserID), OrganizationID: storage.ID(row.OrganizationID),
		TeamID: storage.ID(row.TeamID), Role: row.Role, AddedAt: storage.Time(row.AddedAt),
		DisplayName: display, Email: email,
	}
}
