// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// This file maps the groups an identity provider asserts to roles and teams
// (GOV-004, ADR-0074). A provider has one mapping document. Each sign-in
// makes what the person holds through the provider equal to what the
// document gives their groups: grants and team memberships are added, and
// those the provider gave earlier and the person no longer qualifies for
// are removed. Anything an administrator granted by hand is never touched,
// because only the rows the provider created are recorded as its own
// (federated_assignments).

// defaultGroupsClaim is the claim or attribute that carries groups when the
// mapping names none.
const defaultGroupsClaim = "groups"

// teamMemberRole is the role of a team membership, as in tenancy: a team's
// members hold the roles the team is granted.
const teamMemberRole = "member"

// GrantRule is one grant a mapping rule gives.
type GrantRule struct {
	Role  string `json:"role"`
	Scope Scope  `json:"scope"`
	// App and Environment are keys.
	App         string `json:"app,omitempty"`
	Environment string `json:"environment,omitempty"`
}

// MappingRule gives the members of one group their grants and teams.
type MappingRule struct {
	// Group is the asserted group value, compared exactly.
	Group string `json:"group"`
	// Organization is the key of the organisation the rule acts in; it is
	// required for a provider of the installation and absent for an
	// organisation's own.
	Organization string      `json:"organization,omitempty"`
	Grants       []GrantRule `json:"grants,omitempty"`
	// Teams are team keys.
	Teams []string `json:"teams,omitempty"`
}

// GroupMapping is a provider's mapping document.
type GroupMapping struct {
	// GroupsClaim is the ID-token claim or SAML attribute that carries the
	// groups; empty selects "groups".
	GroupsClaim string        `json:"groupsClaim,omitempty"`
	Rules       []MappingRule `json:"rules"`
}

// claim returns the claim or attribute that carries the groups.
func (m GroupMapping) claim() string {
	if m.GroupsClaim == "" {
		return defaultGroupsClaim
	}
	return m.GroupsClaim
}

// parseMapping reads a stored mapping document.
func parseMapping(raw []byte) (GroupMapping, error) {
	var m GroupMapping
	if len(raw) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return GroupMapping{}, fmt.Errorf("auth: read a group mapping: %w", err)
	}
	return m, nil
}

// GetMapping returns a provider's mapping document.
func (s *Service) GetMapping(ctx context.Context, sc ProviderScope, providerID string) (GroupMapping, error) {
	var out GroupMapping
	err := s.inTx(ctx, sc.tenant(), func(ctx context.Context, tx pgx.Tx) error {
		row, err := loadOwned(ctx, dbgen.New(tx), sc, providerID)
		if err != nil {
			return err
		}
		if out, err = parseMapping(row.Mapping); err != nil {
			return err
		}
		if out.Rules == nil {
			out.Rules = []MappingRule{}
		}
		return nil
	})
	return out, err
}

// SetMapping replaces a provider's mapping document. Every role must exist
// and, in an organisation's own scope, be held by the caller: nobody grants
// more than they have. Every app, environment and team must exist.
func (s *Service) SetMapping(ctx context.Context, sc ProviderScope, providerID string, m GroupMapping) (GroupMapping, error) {
	var org pgtype.UUID
	err := s.inTx(ctx, sc.tenant(), func(ctx context.Context, tx pgx.Tx) error {
		row, err := loadOwned(ctx, dbgen.New(tx), sc, providerID)
		org = row.OrganizationID
		return err
	})
	if err != nil {
		return GroupMapping{}, err
	}
	if err := s.validateMapping(ctx, sc, org, m); err != nil {
		return GroupMapping{}, err
	}
	doc, err := json.Marshal(m)
	if err != nil {
		return GroupMapping{}, fmt.Errorf("auth: encode a group mapping: %w", err)
	}
	if m.Rules == nil {
		m.Rules = []MappingRule{}
	}
	return m, s.inTx(ctx, sc.tenant(), func(ctx context.Context, tx pgx.Tx) error {
		row, err := loadOwned(ctx, dbgen.New(tx), sc, providerID)
		if err != nil {
			return err
		}
		if _, err := dbgen.New(tx).SetIdentityProviderMapping(ctx, dbgen.SetIdentityProviderMappingParams{ID: row.ID, Mapping: doc}); err != nil {
			return fmt.Errorf("auth: set a group mapping: %w", err)
		}
		s.forgetProvider(row.ID)
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: sc.organizationID, Actor: sc.actor, Action: audit.MappingSet,
			TargetKind: "identity_provider", TargetID: providerID, Detail: fmt.Sprintf("rules=%d", len(m.Rules)),
		})
	})
}

// validateMapping checks a document rule by rule, each in the organisation
// it acts in.
func (s *Service) validateMapping(ctx context.Context, sc ProviderScope, providerOrg pgtype.UUID, m GroupMapping) error {
	for i, rule := range m.Rules {
		if rule.Group == "" {
			return plxerr.New(plxerr.InvalidStructure, "rule %d names no group", i+1)
		}
		org, err := s.ruleOrganization(ctx, providerOrg, rule)
		if err != nil {
			return err
		}
		err = s.inTx(ctx, storage.Tenant{OrganizationID: storage.ID(org)}, func(ctx context.Context, tx pgx.Tx) error {
			return s.validateRule(ctx, dbgen.New(tx), sc, org, rule)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// ruleOrganization is the organisation a rule acts in: the provider's, or
// the one a rule of an installation's provider names.
func (s *Service) ruleOrganization(ctx context.Context, providerOrg pgtype.UUID, rule MappingRule) (pgtype.UUID, error) {
	if providerOrg.Valid {
		if rule.Organization != "" {
			return pgtype.UUID{}, plxerr.New(plxerr.InvalidStructure, "a rule of an organisation's provider names no organisation")
		}
		return providerOrg, nil
	}
	if rule.Organization == "" {
		return pgtype.UUID{}, plxerr.New(plxerr.InvalidStructure, "a rule of an installation's provider names the organisation it acts in")
	}
	var org pgtype.UUID
	err := s.inTx(ctx, storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetOrganizationByKey(ctx, rule.Organization)
		if err != nil {
			return notFound(err, "organisation")
		}
		org = row.ID
		return nil
	})
	return org, err
}

// validateRule checks one rule inside its organisation.
func (s *Service) validateRule(ctx context.Context, q *dbgen.Queries, sc ProviderScope, org pgtype.UUID, rule MappingRule) error {
	for _, g := range rule.Grants {
		if err := mappingRole(ctx, q, sc, org, g.Role); err != nil {
			return err
		}
		if err := scopeShape(GrantSpec{Scope: g.Scope, AppID: g.App, EnvironmentID: g.Environment}); err != nil {
			return err
		}
		if _, _, found, err := resolveRuleScope(ctx, q, g); err != nil {
			return err
		} else if !found {
			return plxerr.New(plxerr.ScopeInvalid, "the organisation has no app %q with environment %q", g.App, g.Environment)
		}
	}
	for _, key := range rule.Teams {
		if _, err := q.GetTeamByKey(ctx, key); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return plxerr.New(plxerr.ResourceNotFound, "the organisation has no team %q", key)
			}
			return fmt.Errorf("auth: read a team: %w", err)
		}
	}
	return nil
}

// mappingRole checks that a role a rule grants exists and, for an
// organisation's administrator, that they hold everything it holds.
func mappingRole(ctx context.Context, q *dbgen.Queries, sc ProviderScope, org pgtype.UUID, role string) error {
	if sc.principal != nil {
		return roleHeld(ctx, q, *sc.principal, role)
	}
	if IsPredefined(Role(role)) {
		return nil
	}
	if _, err := q.GetCustomRole(ctx, dbgen.GetCustomRoleParams{OrganizationID: org, Key: role}); err != nil {
		return roleNotFound(err, role)
	}
	return nil
}

// resolveRuleScope turns the app and environment keys of a grant into
// identifiers. found is false when a key names nothing in the organisation.
func resolveRuleScope(ctx context.Context, q *dbgen.Queries, g GrantRule) (app, env pgtype.UUID, found bool, err error) {
	if g.Scope == ScopeOrganization {
		return app, env, true, nil
	}
	row, err := q.GetAppByKey(ctx, g.App)
	if errors.Is(err, pgx.ErrNoRows) {
		return app, env, false, nil
	}
	if err != nil {
		return app, env, false, fmt.Errorf("auth: read an app: %w", err)
	}
	if g.Scope == ScopeApp {
		return row.ID, env, true, nil
	}
	e, err := q.GetEnvironmentByKey(ctx, dbgen.GetEnvironmentByKeyParams{AppID: row.ID, Key: g.Environment})
	if errors.Is(err, pgx.ErrNoRows) {
		return app, env, false, nil
	}
	if err != nil {
		return app, env, false, fmt.Errorf("auth: read an environment: %w", err)
	}
	return row.ID, e.ID, true, nil
}

// wantedGrant is a grant a mapping asks for, with its scope resolved.
type wantedGrant struct {
	role  string
	scope Scope
	app   pgtype.UUID
	env   pgtype.UUID
}

// grantKey identifies a grant within an organisation and holder.
func grantKey(role, scope string, app, env pgtype.UUID) string {
	return role + "|" + scope + "|" + storage.ID(app) + "|" + storage.ID(env)
}

// providerActor is how the audit log names a provider acting on its own.
func providerActor(row dbgen.IdentityProvider) audit.Actor {
	return audit.Actor{Kind: "identity_provider", ID: storage.ID(row.ID), Display: "provider " + row.Key}
}

// assignmentsByOrg groups assignments by organisation.
func assignmentsByOrg(rows []dbgen.ListFederatedAssignmentsOfUserRow) map[string][]dbgen.ListFederatedAssignmentsOfUserRow {
	out := map[string][]dbgen.ListFederatedAssignmentsOfUserRow{}
	for _, r := range rows {
		k := storage.ID(r.OrganizationID)
		out[k] = append(out[k], r)
	}
	return out
}

// applyMapping makes what a provider has given a person equal to what its
// mapping gives their groups now.
func (s *Service) applyMapping(ctx context.Context, prov dbgen.IdentityProvider, user pgtype.UUID, groups []string) error {
	m, err := parseMapping(prov.Mapping)
	if err != nil {
		return err
	}
	byOrg := map[string][]MappingRule{}
	var existing map[string][]dbgen.ListFederatedAssignmentsOfUserRow
	err = s.inTx(ctx, storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		for _, rule := range m.Rules {
			if !slices.Contains(groups, rule.Group) {
				continue
			}
			org := prov.OrganizationID
			if !org.Valid {
				row, err := q.GetOrganizationByKey(ctx, rule.Organization)
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				if err != nil {
					return fmt.Errorf("auth: read an organisation: %w", err)
				}
				org = row.ID
			}
			byOrg[storage.ID(org)] = append(byOrg[storage.ID(org)], rule)
		}
		rows, err := q.ListFederatedAssignmentsOfUser(ctx, dbgen.ListFederatedAssignmentsOfUserParams{ProviderID: prov.ID, UserID: user})
		if err != nil {
			return fmt.Errorf("auth: read the provider's assignments: %w", err)
		}
		existing = assignmentsByOrg(rows)
		return nil
	})
	if err != nil {
		return err
	}
	orgs := map[string]bool{}
	for org := range byOrg {
		orgs[org] = true
	}
	for org := range existing {
		orgs[org] = true
	}
	for _, org := range slices.Sorted(mapKeys(orgs)) {
		if err := s.reconcileOrg(ctx, prov, user, org, byOrg[org], existing[org]); err != nil {
			return err
		}
	}
	return nil
}

// mapKeys iterates over the keys of a set.
func mapKeys(m map[string]bool) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// desiredIn resolves the rules that apply in an organisation to the grants
// and teams they ask for. What no longer resolves (a deleted app, a team)
// is left out: a missing grant is the safe outcome.
func desiredIn(ctx context.Context, q *dbgen.Queries, rules []MappingRule) (map[string]wantedGrant, map[string]pgtype.UUID, error) {
	grants := map[string]wantedGrant{}
	teams := map[string]pgtype.UUID{}
	for _, rule := range rules {
		for _, g := range rule.Grants {
			app, env, found, err := resolveRuleScope(ctx, q, g)
			if err != nil {
				return nil, nil, err
			}
			if found {
				grants[grantKey(g.Role, string(g.Scope), app, env)] = wantedGrant{role: g.Role, scope: g.Scope, app: app, env: env}
			}
		}
		for _, key := range rule.Teams {
			t, err := q.GetTeamByKey(ctx, key)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, nil, fmt.Errorf("auth: read a team: %w", err)
			}
			teams[storage.ID(t.ID)] = t.ID
		}
	}
	return grants, teams, nil
}

// reconcileOrg brings one organisation's assignments of a person in line
// with the rules that apply there.
func (s *Service) reconcileOrg(ctx context.Context, prov dbgen.IdentityProvider, user pgtype.UUID, orgID string, rules []MappingRule, existing []dbgen.ListFederatedAssignmentsOfUserRow) error {
	return s.inTxChanging(ctx, storage.Tenant{OrganizationID: orgID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		grants, teams, err := desiredIn(ctx, q, rules)
		if err != nil {
			return err
		}
		have := map[string]bool{}
		for _, a := range existing {
			kept, err := s.keepOrDrop(ctx, tx, prov, user, a, grants, teams)
			if err != nil {
				return err
			}
			if kept != "" {
				have[kept] = true
			}
		}
		for key, g := range grants {
			if !have["g|"+key] {
				if err := s.addGrant(ctx, tx, prov, user, orgID, g); err != nil {
					return err
				}
			}
		}
		for key, team := range teams {
			if !have["t|"+key] {
				if err := s.addTeam(ctx, tx, prov, user, orgID, team); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// keepOrDrop removes an assignment the mapping no longer asks for and
// otherwise returns its key, so that it is not added again.
func (s *Service) keepOrDrop(ctx context.Context, tx pgx.Tx, prov dbgen.IdentityProvider, user pgtype.UUID, a dbgen.ListFederatedAssignmentsOfUserRow, grants map[string]wantedGrant, teams map[string]pgtype.UUID) (string, error) {
	q := dbgen.New(tx)
	org := storage.ID(a.OrganizationID)
	if a.RoleGrantID.Valid {
		key := grantKey(deref(a.GrantRole), deref(a.GrantScope), a.GrantAppID, a.GrantEnvironmentID)
		if _, ok := grants[key]; ok {
			return "g|" + key, nil
		}
		if _, err := q.DeleteRoleGrant(ctx, a.RoleGrantID); err != nil {
			return "", fmt.Errorf("auth: remove a mapped grant: %w", err)
		}
		return "", s.record(ctx, tx, audit.Entry{
			OrganizationID: org, Actor: providerActor(prov), Action: audit.RoleRevoked,
			TargetKind: "grant", TargetID: storage.ID(a.RoleGrantID), Detail: "user " + storage.ID(user) + " no longer qualifies",
		})
	}
	key := storage.ID(a.TeamID)
	if _, ok := teams[key]; ok {
		return "t|" + key, nil
	}
	if _, err := q.DeleteMembershipByID(ctx, a.MembershipID); err != nil {
		return "", fmt.Errorf("auth: remove a mapped team membership: %w", err)
	}
	return "", s.record(ctx, tx, audit.Entry{
		OrganizationID: org, Actor: providerActor(prov), Action: audit.MemberRemoved,
		TargetKind: "user", TargetID: storage.ID(user), Detail: "team " + key + ": no longer qualifies",
	})
}

// addGrant gives a person a mapped grant and records it as the provider's.
func (s *Service) addGrant(ctx context.Context, tx pgx.Tx, prov dbgen.IdentityProvider, user pgtype.UUID, orgID string, g wantedGrant) error {
	q := dbgen.New(tx)
	id, err := s.newID()
	if err != nil {
		return err
	}
	row, err := q.CreateFederatedGrant(ctx, dbgen.CreateFederatedGrantParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(orgID), UserID: user,
		Role: g.role, Scope: string(g.scope), AppID: g.app, EnvironmentID: g.env,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // the person holds it already, by hand; it stays theirs
	}
	if err != nil {
		return fmt.Errorf("auth: give a mapped grant: %w", err)
	}
	if err := s.assign(ctx, q, prov, orgID, user, row.ID, pgtype.UUID{}); err != nil {
		return err
	}
	return s.record(ctx, tx, audit.Entry{
		OrganizationID: orgID, Actor: providerActor(prov), Action: audit.RoleGranted,
		TargetKind: "grant", TargetID: id, Detail: fmt.Sprintf("user %s role %s scope %s via mapping", storage.ID(user), g.role, g.scope),
	})
}

// addTeam makes a person a member of a mapped team and records it as the
// provider's.
func (s *Service) addTeam(ctx context.Context, tx pgx.Tx, prov dbgen.IdentityProvider, user pgtype.UUID, orgID string, team pgtype.UUID) error {
	q := dbgen.New(tx)
	id, err := s.newID()
	if err != nil {
		return err
	}
	row, err := q.CreateFederatedTeamMembership(ctx, dbgen.CreateFederatedTeamMembershipParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(orgID), UserID: user, TeamID: team, Role: teamMemberRole,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("auth: add a mapped team member: %w", err)
	}
	if err := s.assign(ctx, q, prov, orgID, user, pgtype.UUID{}, row.ID); err != nil {
		return err
	}
	return s.record(ctx, tx, audit.Entry{
		OrganizationID: orgID, Actor: providerActor(prov), Action: audit.MemberAdded,
		TargetKind: "user", TargetID: storage.ID(user), Detail: "team " + storage.ID(team) + " via mapping",
	})
}

// assign records that a grant or membership belongs to a provider.
func (s *Service) assign(ctx context.Context, q *dbgen.Queries, prov dbgen.IdentityProvider, orgID string, user, grant, membership pgtype.UUID) error {
	id, err := s.newID()
	if err != nil {
		return err
	}
	if err := q.CreateFederatedAssignment(ctx, dbgen.CreateFederatedAssignmentParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(orgID), ProviderID: prov.ID, UserID: user,
		RoleGrantID: grant, MembershipID: membership,
	}); err != nil {
		return fmt.Errorf("auth: record a mapped assignment: %w", err)
	}
	return nil
}

// dropProviderAssignments removes everything a provider's mapping gave, in
// every organisation, before the provider is deleted.
func (s *Service) dropProviderAssignments(ctx context.Context, provider pgtype.UUID) error {
	byOrg := map[string][]dbgen.ListFederatedAssignmentsOfProviderRow{}
	err := s.inTx(ctx, storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListFederatedAssignmentsOfProvider(ctx, provider)
		if err != nil {
			return fmt.Errorf("auth: read the provider's assignments: %w", err)
		}
		for _, r := range rows {
			byOrg[storage.ID(r.OrganizationID)] = append(byOrg[storage.ID(r.OrganizationID)], r)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for org, rows := range byOrg {
		err := s.inTxChanging(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
			q := dbgen.New(tx)
			for _, r := range rows {
				if r.RoleGrantID.Valid {
					_, err := q.DeleteRoleGrant(ctx, r.RoleGrantID)
					if err != nil {
						return fmt.Errorf("auth: remove a mapped grant: %w", err)
					}
				} else if _, err := q.DeleteMembershipByID(ctx, r.MembershipID); err != nil {
					return fmt.Errorf("auth: remove a mapped team membership: %w", err)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
