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

	"github.com/nightCode42/plux3/backend/ext/identity"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// This file is the groups half of the SCIM directory: a SCIM group is a team
// of the organisation that SCIM made, and its members are the team's
// members. The roles a group's members hold are the roles the team is
// granted (ADR-0073); SCIM never grants a role.

// teamKey derives a team key from a group's display name: lower-case
// letters, digits and hyphens, at most 63 characters, never empty.
func teamKey(name string) string {
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	key := strings.TrimRight(b.String(), "-")
	if len(key) > 56 {
		key = strings.TrimRight(key[:56], "-")
	}
	if key == "" {
		return "group"
	}
	return key
}

// groupView builds the SCIM group of a team.
func groupView(ctx context.Context, q *dbgen.Queries, org, team pgtype.UUID, ext, name string, version int64, created, modified time.Time) (identity.Group, error) {
	members, err := q.ListTeamMembersDetailed(ctx, dbgen.ListTeamMembersDetailedParams{OrganizationID: org, TeamID: team})
	if err != nil {
		return identity.Group{}, fmt.Errorf("auth: read a group's members: %w", err)
	}
	g := identity.Group{ID: storage.ID(team), ExternalID: ext, DisplayName: name, Version: version, Created: created, Modified: modified}
	for _, m := range members {
		g.Members = append(g.Members, identity.Member{UserID: storage.ID(m.UserID), Display: m.Email})
	}
	return g, nil
}

// loadGroup reads one group.
func loadGroup(ctx context.Context, q *dbgen.Queries, org, team pgtype.UUID) (identity.Group, error) {
	row, err := q.GetScimGroup(ctx, dbgen.GetScimGroupParams{OrganizationID: org, TeamID: team})
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Group{}, identity.ErrNotFound
	}
	if err != nil {
		return identity.Group{}, fmt.Errorf("auth: read a SCIM group: %w", err)
	}
	return groupView(ctx, q, org, team, row.ExternalID, row.DisplayName, row.Version, storage.Time(row.CreatedAt), storage.Time(row.UpdatedAt))
}

// GetGroup returns one group.
func (d scimDirectory) GetGroup(ctx context.Context, org, id string) (identity.Group, error) {
	team, err := resource(id)
	if err != nil {
		return identity.Group{}, err
	}
	var out identity.Group
	err = d.s.inTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = loadGroup(ctx, dbgen.New(tx), storage.MustUUID(org), team)
		return err
	})
	return out, err
}

// ListGroups lists a page of groups, optionally those with one attribute
// equal to a value.
func (d scimDirectory) ListGroups(ctx context.Context, org string, q identity.ListQuery) (identity.GroupPage, error) {
	if err := checkFilter(q.Attribute, "externalId", "displayName"); err != nil {
		return identity.GroupPage{}, err
	}
	size, offset := d.page(q)
	var out identity.GroupPage
	err := d.s.inTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		queries, orgID := dbgen.New(tx), storage.MustUUID(org)
		rows, err := queries.ListScimGroups(ctx, dbgen.ListScimGroupsParams{
			OrganizationID: orgID, Attribute: q.Attribute, Value: q.Value, PageSize: size, PageOffset: offset,
		})
		if err != nil {
			return fmt.Errorf("auth: list SCIM groups: %w", err)
		}
		for _, r := range rows {
			g, err := groupView(ctx, queries, orgID, r.TeamID, r.ExternalID, r.DisplayName, r.Version, storage.Time(r.CreatedAt), storage.Time(r.UpdatedAt))
			if err != nil {
				return err
			}
			out.Groups = append(out.Groups, g)
			out.Total = int(r.Total)
		}
		if len(rows) == 0 {
			n, err := queries.CountScimGroups(ctx, dbgen.CountScimGroupsParams{OrganizationID: orgID, Attribute: q.Attribute, Value: q.Value})
			if err != nil {
				return fmt.Errorf("auth: count SCIM groups: %w", err)
			}
			out.Total = int(n)
		}
		return nil
	})
	return out, err
}

// memberIDs parses and checks the members of a group: each must be a person
// the organisation's SCIM client provisioned and has not deleted.
func memberIDs(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, ids []string) ([]pgtype.UUID, error) {
	out := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		user, err := storage.UUID(id)
		if err != nil {
			return nil, invalidValue("the member %q is not a user identifier", id)
		}
		if _, err := q.GetScimUserWithAccount(ctx, dbgen.GetScimUserWithAccountParams{OrganizationID: org, UserID: user}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, invalidValue("the member %q is not a user of this directory", id)
			}
			return nil, fmt.Errorf("auth: read a SCIM user: %w", err)
		}
		if !slices.ContainsFunc(out, func(u pgtype.UUID) bool { return u == user }) {
			out = append(out, user)
		}
	}
	return out, nil
}

// CreateGroup makes a team for a group and adds its members.
func (d scimDirectory) CreateGroup(ctx context.Context, org string, spec identity.GroupSpec) (identity.Group, error) {
	if err := scimWrite(ctx); err != nil {
		return identity.Group{}, err
	}
	name := strings.TrimSpace(spec.DisplayName)
	if name == "" || len(name) > 120 {
		return identity.Group{}, invalidValue("displayName is required and at most 120 characters")
	}
	var out identity.Group
	err := d.s.inTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		q, orgID := dbgen.New(tx), storage.MustUUID(org)
		members, err := memberIDs(ctx, q, orgID, spec.MemberIDs)
		if err != nil {
			return err
		}
		team, err := d.s.createTeamFor(ctx, q, org, name)
		if err != nil {
			return err
		}
		if _, err := q.CreateScimGroup(ctx, dbgen.CreateScimGroupParams{OrganizationID: orgID, TeamID: team.ID, ExternalID: spec.ExternalID}); err != nil {
			return scimWriteError(err, "create a SCIM group")
		}
		if err := d.s.setMembers(ctx, q, orgID, team.ID, members); err != nil {
			return err
		}
		if out, err = loadGroup(ctx, q, orgID, team.ID); err != nil {
			return err
		}
		return d.s.record(ctx, tx, audit.Entry{
			OrganizationID: org, Actor: scimActor, Action: audit.ScimGroupCreated,
			TargetKind: "team", TargetID: out.ID, Detail: fmt.Sprintf("members=%d", len(members)),
		})
	})
	return out, err
}

// createTeamFor makes the team of a group. Its key comes from the name; a
// key already taken gets the end of the new team identifier appended.
func (s *Service) createTeamFor(ctx context.Context, q *dbgen.Queries, org, name string) (dbgen.Team, error) {
	id, err := s.newID()
	if err != nil {
		return dbgen.Team{}, err
	}
	key := teamKey(name)
	if _, err := q.GetTeamByKey(ctx, key); err == nil {
		plain := strings.ReplaceAll(id, "-", "")
		key += "-" + plain[len(plain)-6:]
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Team{}, fmt.Errorf("auth: read a team: %w", err)
	}
	team, err := q.CreateTeam(ctx, dbgen.CreateTeamParams{ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(org), Key: key, Name: name})
	if err != nil {
		return dbgen.Team{}, scimWriteError(err, "create a team")
	}
	return team, nil
}

// setMembers makes the members of a team exactly the given users.
func (s *Service) setMembers(ctx context.Context, q *dbgen.Queries, org, team pgtype.UUID, want []pgtype.UUID) error {
	have, err := q.ListTeamMembersDetailed(ctx, dbgen.ListTeamMembersDetailedParams{OrganizationID: org, TeamID: team})
	if err != nil {
		return fmt.Errorf("auth: read a team's members: %w", err)
	}
	for _, m := range have {
		if slices.ContainsFunc(want, func(u pgtype.UUID) bool { return u == m.UserID }) {
			continue
		}
		if _, err := q.RemoveMembership(ctx, dbgen.RemoveMembershipParams{OrganizationID: org, UserID: m.UserID, TeamID: team}); err != nil {
			return fmt.Errorf("auth: remove a team member: %w", err)
		}
	}
	for _, user := range want {
		if slices.ContainsFunc(have, func(m dbgen.ListTeamMembersDetailedRow) bool { return m.UserID == user }) {
			continue
		}
		id, err := s.newID()
		if err != nil {
			return err
		}
		if _, err := q.AddTeamMembership(ctx, dbgen.AddTeamMembershipParams{
			ID: storage.MustUUID(id), OrganizationID: org, UserID: user, TeamID: team, Role: teamMemberRole,
		}); err != nil {
			return fmt.Errorf("auth: add a team member: %w", err)
		}
	}
	return nil
}

// ReplaceGroup sets a group's name, external identifier and members.
func (d scimDirectory) ReplaceGroup(ctx context.Context, org, id string, spec identity.GroupSpec, ifVersion int64) (identity.Group, error) {
	if err := scimWrite(ctx); err != nil {
		return identity.Group{}, err
	}
	team, err := resource(id)
	if err != nil {
		return identity.Group{}, err
	}
	name := strings.TrimSpace(spec.DisplayName)
	if name == "" || len(name) > 120 {
		return identity.Group{}, invalidValue("displayName is required and at most 120 characters")
	}
	var out identity.Group
	err = d.s.inTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		q, orgID := dbgen.New(tx), storage.MustUUID(org)
		rec, err := q.GetScimGroupForUpdate(ctx, dbgen.GetScimGroupForUpdateParams{OrganizationID: orgID, TeamID: team})
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("auth: read a SCIM group: %w", err)
		}
		if ifVersion != 0 && ifVersion != rec.Version {
			return identity.ErrVersionMismatch
		}
		members, err := memberIDs(ctx, q, orgID, spec.MemberIDs)
		if err != nil {
			return err
		}
		// Removing members can remove the last owner, through a team that
		// holds the owner role.
		err = GuardOwners(ctx, q, orgID, func() error {
			if _, err := q.UpdateTeam(ctx, dbgen.UpdateTeamParams{ID: team, Name: name}); err != nil {
				return fmt.Errorf("auth: rename a team: %w", err)
			}
			return d.s.setMembers(ctx, q, orgID, team, members)
		})
		if errors.Is(err, plxerr.Sentinel(plxerr.LastOwnerCannotBeRemoved)) {
			return refused("the change would leave the organisation without an owner")
		}
		if err != nil {
			return err
		}
		if _, err := q.UpdateScimGroup(ctx, dbgen.UpdateScimGroupParams{OrganizationID: orgID, TeamID: team, ExternalID: spec.ExternalID}); err != nil {
			return scimWriteError(err, "update a SCIM group")
		}
		if out, err = loadGroup(ctx, q, orgID, team); err != nil {
			return err
		}
		return d.s.record(ctx, tx, audit.Entry{
			OrganizationID: org, Actor: scimActor, Action: audit.ScimGroupChanged,
			TargetKind: "team", TargetID: id, Detail: fmt.Sprintf("members=%d", len(members)),
		})
	})
	return out, err
}

// DeleteGroup removes a group and its team; its members keep their accounts.
func (d scimDirectory) DeleteGroup(ctx context.Context, org, id string) error {
	if err := scimWrite(ctx); err != nil {
		return err
	}
	team, err := resource(id)
	if err != nil {
		return err
	}
	return d.s.inTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		q, orgID := dbgen.New(tx), storage.MustUUID(org)
		if _, err := q.GetScimGroupForUpdate(ctx, dbgen.GetScimGroupForUpdateParams{OrganizationID: orgID, TeamID: team}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return identity.ErrNotFound
			}
			return fmt.Errorf("auth: read a SCIM group: %w", err)
		}
		err := GuardOwners(ctx, q, orgID, func() error {
			if _, err := q.DeleteTeam(ctx, team); err != nil {
				return fmt.Errorf("auth: delete a team: %w", err)
			}
			return nil
		})
		if errors.Is(err, plxerr.Sentinel(plxerr.LastOwnerCannotBeRemoved)) {
			return refused("the change would leave the organisation without an owner")
		}
		if err != nil {
			return err
		}
		return d.s.record(ctx, tx, audit.Entry{
			OrganizationID: org, Actor: scimActor, Action: audit.ScimGroupDeleted, TargetKind: "team", TargetID: id,
		})
	})
}
