// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// ProposedID identifies a proposed grant or restriction in an explanation:
// it is evaluated but not saved, so it has no identifier of its own.
const ProposedID = "proposed"

// ProposedGrant is a grant an explanation evaluates without saving it.
type ProposedGrant struct {
	// UserID or TeamID, exactly one, is who would hold the role.
	UserID, TeamID string
	Role           string
	Scope          Scope
	// AppID and Environment (the environment's key) are as the scope needs.
	AppID, Environment string
}

// AccessQuery asks why a person can or cannot use a permission in a scope
// (GOV-007).
type AccessQuery struct {
	// UserID is the person; "" is the caller.
	UserID     string
	Permission Permission
	Scope      AccessScope
	// Grant or Restriction, at most one, is a change evaluated on top of the
	// stored state; nothing is saved.
	Grant       *ProposedGrant
	Restriction *RestrictionSpec
}

// AccessDecision is the answer to an AccessQuery.
type AccessDecision struct {
	Allowed bool
	// AllowingGrants are the grants, memberships and app access entries
	// whose roles carry the permission in the scope; none means no grant.
	AllowingGrants []string
	// RemovingRestrictions are the restrictions that take it away in the
	// scope.
	RemovingRestrictions []string
	// OwnerFloor is set when a restriction names the permission and the
	// person's owner floor keeps it (GOV-006).
	OwnerFloor bool
}

// ExplainAccess says why a person can or cannot use a permission in a scope,
// naming the grants that allow it and the restrictions that remove it, by
// running the resolver that every call runs on the stored state, or on it
// plus the proposed grant or restriction (GOV-007). It never writes.
// Anyone may explain their own access; explaining another person's needs
// members.manage.
func (s *Service) ExplainAccess(ctx context.Context, p Principal, q AccessQuery) (AccessDecision, error) {
	person := q.UserID
	if person == "" {
		person = p.UserID
	}
	if person != p.UserID {
		if err := p.Authorize(MembersManage); err != nil {
			return AccessDecision{}, err
		}
	}
	if err := checkAccessQuery(q, person); err != nil {
		return AccessDecision{}, err
	}
	user, err := parseID(person, "user")
	if err != nil {
		return AccessDecision{}, err
	}
	var state accessState
	err = s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		state, err = s.readAccessState(ctx, dbgen.New(tx), storage.MustUUID(p.OrganizationID), user, q)
		return err
	})
	if err != nil {
		if errors.Is(err, plxerr.Sentinel(plxerr.ResourceNotFound)) && person != p.UserID {
			return AccessDecision{}, plxerr.New(plxerr.ResourceNotFound, "no such member of the organisation")
		}
		return AccessDecision{}, err
	}
	return state.decide(person, q.Permission, q.Scope), nil
}

// checkAccessQuery validates what a query names before anything is read.
func checkAccessQuery(q AccessQuery, person string) error {
	if person == "" {
		return plxerr.New(plxerr.InvalidStructure, "this credential belongs to no person; name the person to explain")
	}
	if _, err := ParsePermission(string(q.Permission)); err != nil {
		return err
	}
	if q.Grant != nil && q.Restriction != nil {
		return plxerr.New(plxerr.InvalidStructure, "propose a grant or a restriction, not both")
	}
	return q.Scope.validate()
}

// accessState is what the resolver reads to decide for one person, with a
// proposal added in memory.
type accessState struct {
	given        []roleGrant
	custom       map[Role][]ScopedPermission
	restrictions []Restriction
}

// readAccessState reads the stored state for a person, and adds the proposal
// to it. It reads only.
func (s *Service) readAccessState(ctx context.Context, q *dbgen.Queries, org, user pgtype.UUID, query AccessQuery) (accessState, error) {
	loaded, err := s.readGrants(ctx, q, org, user)
	if err != nil {
		return accessState{}, err
	}
	state := accessState{given: loaded.given, custom: loaded.custom, restrictions: loaded.restrictions}
	if query.Grant == nil && query.Restriction == nil {
		return state, nil
	}
	teams, err := teamsOf(ctx, q, org, user)
	if err != nil {
		return accessState{}, err
	}
	switch {
	case query.Grant != nil:
		err = s.addProposedGrant(ctx, q, org, &state, storage.ID(user), teams, *query.Grant)
	case query.Restriction != nil:
		err = addProposedRestriction(&state, storage.ID(user), teams, *query.Restriction)
	}
	return state, err
}

// addProposedGrant adds a proposed grant to the state when it would apply
// to the person, who is the grant's user or belongs to its team.
func (s *Service) addProposedGrant(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, state *accessState, person string, teams []string, proposal ProposedGrant) error {
	if (proposal.UserID == "") == (proposal.TeamID == "") {
		return plxerr.New(plxerr.InvalidStructure, "a grant names exactly one of a team and a user")
	}
	if err := scopeShape(GrantSpec{Scope: proposal.Scope, AppID: proposal.AppID, EnvironmentID: proposal.Environment}); err != nil {
		return err
	}
	if !IsPredefined(Role(proposal.Role)) {
		if _, err := q.GetCustomRole(ctx, dbgen.GetCustomRoleParams{OrganizationID: org, Key: proposal.Role}); err != nil {
			return roleNotFound(err, proposal.Role)
		}
	}
	if proposal.UserID != person && !slices.Contains(teams, proposal.TeamID) {
		return nil
	}
	state.given = append(slices.Clone(state.given), roleGrant{
		ID: ProposedID, Role: Role(proposal.Role), Scope: proposal.Scope, AppID: proposal.AppID, Environment: proposal.Environment,
	})
	custom, err := s.customRoleTable(ctx, q, org, state.given)
	if err != nil {
		return err
	}
	state.custom = custom
	return nil
}

// addProposedRestriction adds a proposed restriction to the state when it
// would apply to the person.
func addProposedRestriction(state *accessState, person string, teams []string, spec RestrictionSpec) error {
	r, err := restrictionOf(ProposedID, spec)
	if err != nil {
		return err
	}
	if appliesTo(r, person, teams) {
		state.restrictions = append(slices.Clone(state.restrictions), r)
	}
	return nil
}

// decide runs the resolver on the state and says what it finds: whether the
// person holds the permission in the scope, which grants carry it, and which
// restrictions take it away.
func (st accessState) decide(person string, perm Permission, scope AccessScope) AccessDecision {
	resolved := resolve(Identity{UserID: person}, "", st.given, st.custom, nil, st.restrictions...)
	decision := AccessDecision{Allowed: resolved.holdsIn(perm, scope)}
	for _, g := range st.given {
		if resolve(Identity{}, "", []roleGrant{g}, st.custom, nil).holdsIn(perm, scope) {
			decision.AllowingGrants = append(decision.AllowingGrants, g.ID)
		}
	}
	if len(decision.AllowingGrants) == 0 {
		return decision
	}
	floor := slices.Contains(ownerFloorFor(st.given), perm)
	for _, r := range resolved.Restrictions {
		switch {
		case !r.coversScope(scope):
		case slices.Contains(r.Removed, perm):
			decision.RemovingRestrictions = append(decision.RemovingRestrictions, r.ID)
		case floor && r.removes(perm):
			decision.OwnerFloor = true
		}
	}
	return decision
}

// coversScope reports whether a restriction applies in a scope: one on the
// whole organisation everywhere, one on an app on that app and its
// environments, one on an environment there.
func (r Restriction) coversScope(scope AccessScope) bool {
	switch {
	case r.AppID == "":
		return true
	case r.AppID != scope.AppID:
		return false
	}
	return r.Environment == "" || r.Environment == scope.Environment
}
