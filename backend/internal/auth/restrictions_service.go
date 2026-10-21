// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// RestrictionSpec is what a new restriction says (GOV-006).
type RestrictionSpec struct {
	SubjectKind SubjectKind
	// SubjectID is the team or the user; "" for the organisation.
	SubjectID string
	// Patterns are catalogue permissions or the wildcards "*", "<area>.*"
	// and "<area>.<feature>.*".
	Patterns []string
	// AppID and Environment limit the restriction; see Restriction.
	AppID, Environment string
	Reason             string
}

// RestrictionFilter narrows a list of restrictions.
type RestrictionFilter struct {
	// SubjectID is a team or a user.
	SubjectID string
	AppID     string
}

// restrictionRecord converts a stored restriction.
func restrictionRecord(row dbgen.RestrictionPolicy) Restriction {
	return Restriction{
		ID: storage.ID(row.ID), SubjectKind: SubjectKind(row.SubjectKind), SubjectID: storage.ID(row.SubjectID),
		Patterns: slices.Clone(row.Patterns), AppID: storage.ID(row.AppID), Environment: deref(row.Environment),
		Reason: row.Reason, CreatedBy: storage.ID(row.CreatedBy), CreatedAt: storage.Time(row.CreatedAt),
	}
}

// restrictionsApplying reads the restrictions that apply to a person in an
// organisation: the organisation's, those of the teams the person belongs
// to, and their own. With no person only the organisation's apply.
func restrictionsApplying(ctx context.Context, q *dbgen.Queries, org, user pgtype.UUID) ([]Restriction, error) {
	rows, err := q.ListRestrictionsForPerson(ctx, dbgen.ListRestrictionsForPersonParams{OrganizationID: org, UserID: user})
	if err != nil {
		return nil, fmt.Errorf("auth: read restrictions: %w", err)
	}
	out := make([]Restriction, len(rows))
	for i, row := range rows {
		out[i] = restrictionRecord(row)
	}
	return out, nil
}

// restrictionsOf is restrictionsApplying in a transaction of its own, for an
// identity that has no grants to read, such as a workload.
func (s *Service) restrictionsOf(ctx context.Context, organizationID string, org, user pgtype.UUID) ([]Restriction, error) {
	var out []Restriction
	err := s.inTx(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) (err error) {
		out, err = restrictionsApplying(ctx, dbgen.New(tx), org, user)
		return err
	})
	return out, err
}

// PermissionsVersion returns the organisation's permissions version, which
// rises in the transaction of every change to grants, roles, memberships
// and restrictions (ADR-0086).
func (s *Service) PermissionsVersion(ctx context.Context, organizationID string) (int64, error) {
	org, err := parseID(organizationID, "organisation")
	if err != nil {
		return 0, err
	}
	var version int64
	err = s.inTx(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) (err error) {
		version, err = dbgen.New(tx).GetPermissionsVersion(ctx, org)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("auth: read the permissions version: %w", err)
	}
	return version, nil
}

// restrictionLimits returns the limits in force for the organisation's
// restrictions: the installation's, tightened by the organisation's own
// overrides, which can only lower them.
func (s *Service) restrictionLimits(ctx context.Context, q *dbgen.Queries, org pgtype.UUID) (limits.Set, error) {
	set := s.o.Limits
	rows, err := q.ListLimitOverrides(ctx, dbgen.ListLimitOverridesParams{Scope: "organization", ScopeID: org})
	if err != nil {
		return set, fmt.Errorf("auth: read limit overrides: %w", err)
	}
	for _, row := range rows {
		k := limits.Key(row.LimitKey)
		if k != limits.AuthRestrictionsPerOrg && k != limits.AuthRestrictionsPerSubject && k != limits.AuthRestrictionPatterns {
			continue
		}
		if tightened, err := set.Tighten(k, limits.ScopeOrganization, row.Value); err == nil {
			set = tightened
		}
	}
	return set, nil
}

// restrictionOf validates the shape of a new restriction before anything is
// read: the subject, the patterns against the catalogue (PLX-8104), and that
// an environment comes with an app.
func restrictionOf(id string, spec RestrictionSpec) (Restriction, error) {
	r := Restriction{
		ID: id, SubjectKind: spec.SubjectKind, SubjectID: spec.SubjectID,
		AppID: spec.AppID, Environment: spec.Environment, Reason: strings.TrimSpace(spec.Reason),
	}
	switch spec.SubjectKind {
	case SubjectOrganization:
		if spec.SubjectID != "" {
			return Restriction{}, plxerr.New(plxerr.InvalidStructure, "a restriction on the organisation names no subject")
		}
	case SubjectTeam, SubjectUser:
		if _, err := storage.UUID(spec.SubjectID); err != nil {
			return Restriction{}, plxerr.New(plxerr.InvalidFormat, "the subject identifier is not valid")
		}
	default:
		return Restriction{}, plxerr.New(plxerr.InvalidStructure, "%q is not a subject; the subjects are organization, team and user", spec.SubjectKind)
	}
	if spec.Environment != "" && (spec.AppID == "" || !validKey(spec.Environment)) {
		return Restriction{}, plxerr.New(plxerr.RestrictionPatternInvalid, "an environment is the key of an environment of the app the restriction names")
	}
	patterns, err := checkPatterns(spec.Patterns)
	if err != nil {
		return Restriction{}, err
	}
	r.Patterns = patterns
	return r, nil
}

// CreateRestriction creates a restriction (GOV-006). It needs
// restrictions.manage with a step-up, and organization.manage as well for a
// restriction on the whole organisation. The caller may restrict only
// permissions they hold in the scope the restriction names, which also keeps
// them from restricting themselves in a scope wider than their own grants
// (PLX-8106). A restriction that would leave no Owner able to manage members,
// roles and restrictions is refused (PLX-8105), as are counts over the
// limits (PLX-8107); a refusal for lockout or escalation is audited. The
// organisation's permissions version rises with the insert.
func (s *Service) CreateRestriction(ctx context.Context, p Principal, spec RestrictionSpec) (Restriction, error) {
	if err := authorizeRestrictionChange(p, spec.SubjectKind); err != nil {
		return Restriction{}, err
	}
	id, err := s.newID()
	if err != nil {
		return Restriction{}, err
	}
	r, err := restrictionOf(id, spec)
	if err != nil {
		return Restriction{}, err
	}
	err = s.inTxRefusing(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		return s.insertRestriction(ctx, tx, p, &r)
	})
	if err != nil {
		return Restriction{}, err
	}
	return r, nil
}

// authorizeRestrictionChange checks the permissions a change of a
// restriction on the subject needs: restrictions.manage, and
// organization.manage for the whole organisation (ADR-0086).
func authorizeRestrictionChange(p Principal, kind SubjectKind) error {
	if err := p.Authorize(RestrictionsManage); err != nil {
		return err
	}
	if kind == SubjectOrganization {
		return p.Authorize(OrganizationManage)
	}
	return nil
}

// insertRestriction runs the checks that read the organisation and writes
// the restriction and its audit entry, in the caller's transaction. r is
// updated with what the database stored.
func (s *Service) insertRestriction(ctx context.Context, tx pgx.Tx, p Principal, r *Restriction) error {
	q := dbgen.New(tx)
	org := storage.MustUUID(p.OrganizationID)
	team, user, err := s.restrictionSubject(ctx, q, org, *r)
	if err != nil {
		return err
	}
	app, err := restrictionScope(ctx, q, *r)
	if err != nil {
		return err
	}
	if err := s.refusing(ctx, tx, p, *r, holdsWhatItRestricts(p, *r)); err != nil {
		return err
	}
	set, err := s.restrictionLimits(ctx, q, org)
	if err != nil {
		return err
	}
	if err := checkRestrictionLimits(ctx, q, org, set, *r); err != nil {
		return err
	}
	if err := s.refusing(ctx, tx, p, *r, guardOwnerReach(ctx, q, org, *r)); err != nil {
		return err
	}
	creator, err := storage.NullUUID(p.UserID)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	var env *string
	if r.Environment != "" {
		env = &r.Environment
	}
	row, err := q.CreateRestriction(ctx, dbgen.CreateRestrictionParams{
		ID: storage.MustUUID(r.ID), OrganizationID: org, SubjectKind: string(r.SubjectKind),
		TeamID: team, UserID: user, Patterns: r.Patterns, AppID: app, Environment: env, Reason: r.Reason, CreatedBy: creator,
	})
	if err != nil {
		return fmt.Errorf("auth: create a restriction: %w", err)
	}
	*r = restrictionRecord(row)
	return s.record(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(),
		Action: audit.RestrictionCreated, TargetKind: "restriction", TargetID: r.ID, Detail: restrictionDetail(*r),
	})
}

// refusing passes a nil error through. A lockout or an escalation is
// recorded in the audit log, and the transaction commits that entry alone
// before the refusal is returned; any other error is returned as it is.
func (s *Service) refusing(ctx context.Context, tx pgx.Tx, p Principal, r Restriction, err error) error {
	if err == nil {
		return nil
	}
	var coded *plxerr.Error
	if !errors.As(err, &coded) || (coded.Code != plxerr.RestrictionLocksOutOwner && coded.Code != plxerr.RestrictionExceedsCaller) {
		return err
	}
	if recordErr := s.record(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(),
		Action: audit.RestrictionRefused, TargetKind: "restriction", TargetID: r.ID,
		Detail: fmt.Sprintf("PLX-%d: %s", coded.Code, restrictionDetail(r)),
	}); recordErr != nil {
		return recordErr
	}
	return refuse(err)
}

// restrictionSubject checks that the team or the person a restriction names
// is of the organisation, and returns it as the column that holds it.
func (*Service) restrictionSubject(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, r Restriction) (team, user pgtype.UUID, err error) {
	switch r.SubjectKind {
	case SubjectTeam:
		team = storage.MustUUID(r.SubjectID)
	case SubjectUser:
		user = storage.MustUUID(r.SubjectID)
	case SubjectOrganization:
		return team, user, nil
	}
	return team, user, holderExists(ctx, q, org, team, user)
}

// restrictionScope checks that the app, and the environment of that app, a
// restriction names exist (PLX-8102), and returns the app.
func restrictionScope(ctx context.Context, q *dbgen.Queries, r Restriction) (pgtype.UUID, error) {
	if r.AppID == "" {
		return pgtype.UUID{}, nil
	}
	app, err := storage.UUID(r.AppID)
	if err != nil {
		return app, plxerr.New(plxerr.ScopeInvalid, "the app identifier is not valid")
	}
	if _, err := q.GetApp(ctx, app); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app, plxerr.New(plxerr.ScopeInvalid, "the organisation has no such app")
		}
		return app, fmt.Errorf("auth: read an app: %w", err)
	}
	if r.Environment == "" {
		return app, nil
	}
	if _, err := q.GetEnvironmentByKey(ctx, dbgen.GetEnvironmentByKeyParams{AppID: app, Key: r.Environment}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app, plxerr.New(plxerr.ScopeInvalid, "the app has no such environment")
		}
		return app, fmt.Errorf("auth: read an environment: %w", err)
	}
	return app, nil
}

// checkRestrictionLimits refuses a restriction that would take the
// organisation or its subject over the registry's counts, or that lists more
// patterns than it allows (PLX-8107, LIM-001).
func checkRestrictionLimits(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, set limits.Set, r Restriction) error {
	if int64(len(r.Patterns)) > set.Get(limits.AuthRestrictionPatterns) {
		return plxerr.New(plxerr.RestrictionLimitReached, "a restriction lists at most %d permissions or wildcards", set.Get(limits.AuthRestrictionPatterns))
	}
	total, err := q.CountRestrictions(ctx, org)
	if err != nil {
		return fmt.Errorf("auth: count restrictions: %w", err)
	}
	if total >= set.Get(limits.AuthRestrictionsPerOrg) {
		return plxerr.New(plxerr.RestrictionLimitReached, "the organisation has %d restrictions, the most it may have", total)
	}
	subject, err := storage.NullUUID(r.SubjectID)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	own, err := q.CountRestrictionsOfSubject(ctx, dbgen.CountRestrictionsOfSubjectParams{OrganizationID: org, SubjectID: subject})
	if err != nil {
		return fmt.Errorf("auth: count the restrictions of a subject: %w", err)
	}
	if own >= set.Get(limits.AuthRestrictionsPerSubject) {
		return plxerr.New(plxerr.RestrictionLimitReached, "the subject has %d restrictions, the most it may have", own)
	}
	return nil
}

// holdsWhatItRestricts refuses a restriction that removes a permission the
// caller does not hold in the scope the restriction names: nobody restricts
// what they do not have (PLX-8106).
func holdsWhatItRestricts(p Principal, r Restriction) error {
	for _, info := range matchedByPatterns(r.Patterns) {
		if !r.removes(info.Name) || holdsInScope(p, info.Name, r) {
			continue
		}
		return plxerr.New(plxerr.RestrictionExceedsCaller,
			"%s is within this restriction and you do not hold it in the scope it names; name only what you hold", info.Name)
	}
	return nil
}

// holdsInScope reports whether the principal holds a permission in the scope
// a restriction names.
func holdsInScope(p Principal, perm Permission, r Restriction) bool {
	switch {
	case r.AppID == "":
		return p.Holds(perm)
	case r.Environment == "":
		return p.HoldsOnApp(perm, r.AppID)
	default:
		return p.HoldsOnEnvironment(perm, r.AppID, r.Environment)
	}
}

// removesOwnerFloor reports whether a restriction takes the owner floor
// away from whom it applies to: only one on the whole organisation can.
func removesOwnerFloor(r Restriction) bool {
	return r.AppID == "" && slices.ContainsFunc(ownerFloor, r.removes)
}

// guardOwnerReach refuses a restriction after which every Owner of the
// organisation would have the owner floor removed from them by a
// restriction, so that nobody could manage members, roles and restrictions
// (PLX-8105). The resolver gives an Owner the floor back whatever the
// restrictions say; this guard keeps such a restriction out of the table in
// the first place. An organisation with no Owner has nobody to lock out.
func guardOwnerReach(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, proposed Restriction) error {
	if !removesOwnerFloor(proposed) {
		return nil
	}
	owners, err := q.ListOwnerUserIDs(ctx, org)
	if err != nil {
		return fmt.Errorf("auth: list owners: %w", err)
	}
	for _, owner := range owners {
		blocked, err := floorRemovedFrom(ctx, q, org, owner, proposed)
		if err != nil {
			return err
		}
		if !blocked {
			return nil
		}
	}
	if len(owners) == 0 {
		return nil
	}
	return plxerr.New(plxerr.RestrictionLocksOutOwner,
		"this restriction would leave no owner able to manage members, roles and restrictions; owners must keep that ability")
}

// floorRemovedFrom reports whether some restriction, among those already
// made and the proposed one, removes the owner floor from the person.
func floorRemovedFrom(ctx context.Context, q *dbgen.Queries, org, person pgtype.UUID, proposed Restriction) (bool, error) {
	existing, err := restrictionsApplying(ctx, q, org, person)
	if err != nil {
		return false, err
	}
	teams, err := teamsOf(ctx, q, org, person)
	if err != nil {
		return false, err
	}
	if appliesTo(proposed, storage.ID(person), teams) {
		existing = append(existing, proposed)
	}
	return slices.ContainsFunc(existing, removesOwnerFloor), nil
}

// teamsOf returns the identifiers of the teams a person belongs to.
func teamsOf(ctx context.Context, q *dbgen.Queries, org, person pgtype.UUID) ([]string, error) {
	rows, err := q.ListMembershipsForUser(ctx, dbgen.ListMembershipsForUserParams{OrganizationID: org, UserID: person})
	if err != nil {
		return nil, fmt.Errorf("auth: read memberships: %w", err)
	}
	var teams []string
	for _, m := range rows {
		if m.TeamID.Valid {
			teams = append(teams, storage.ID(m.TeamID))
		}
	}
	return teams, nil
}

// appliesTo reports whether a restriction applies to a person who belongs to
// the given teams.
func appliesTo(r Restriction, person string, teams []string) bool {
	switch r.SubjectKind {
	case SubjectOrganization:
		return true
	case SubjectUser:
		return r.SubjectID == person
	case SubjectTeam:
		return slices.Contains(teams, r.SubjectID)
	}
	return false
}

// restrictionDetail says whom a restriction is for, what it removes and
// where.
func restrictionDetail(r Restriction) string {
	subject := string(r.SubjectKind)
	if r.SubjectID != "" {
		subject += " " + r.SubjectID
	}
	where := "organization"
	if r.AppID != "" {
		where = "app " + r.AppID
	}
	if r.Environment != "" {
		where += " environment " + r.Environment
	}
	return fmt.Sprintf("%s removes %s on %s", subject, strings.Join(r.Patterns, " "), where)
}

// ListRestrictions lists the organisation's restrictions in identifier
// order (GOV-006). It needs restrictions.manage.
func (s *Service) ListRestrictions(ctx context.Context, p Principal, f RestrictionFilter, after storage.Cursor, limit int32) ([]Restriction, error) {
	if err := p.Authorize(RestrictionsManage); err != nil {
		return nil, err
	}
	subject, err := storage.NullUUID(f.SubjectID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the subject identifier is not valid")
	}
	app, err := storage.NullUUID(f.AppID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	var out []Restriction
	err = s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListRestrictions(ctx, dbgen.ListRestrictionsParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), AfterID: after.AfterID(),
			SubjectID: subject, AppID: app, PageSize: limit,
		})
		if err != nil {
			return fmt.Errorf("auth: list restrictions: %w", err)
		}
		out = make([]Restriction, len(rows))
		for i, row := range rows {
			out[i] = restrictionRecord(row)
		}
		return nil
	})
	return out, err
}

// DeleteRestriction removes a restriction (GOV-006). It needs
// restrictions.manage with a step-up, and organization.manage as well for a
// restriction on the whole organisation, like creating one. The
// organisation's permissions version rises with the delete.
func (s *Service) DeleteRestriction(ctx context.Context, p Principal, restrictionID string) error {
	if err := p.Authorize(RestrictionsManage); err != nil {
		return err
	}
	id, err := parseID(restrictionID, "restriction")
	if err != nil {
		return err
	}
	return s.inTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetRestriction(ctx, id)
		if err != nil {
			return notFound(err, "restriction")
		}
		r := restrictionRecord(row)
		if err := authorizeRestrictionChange(p, r.SubjectKind); err != nil {
			return err
		}
		if _, err := q.DeleteRestriction(ctx, id); err != nil {
			return fmt.Errorf("auth: delete a restriction: %w", err)
		}
		return s.record(ctx, tx, audit.Entry{
			OrganizationID: p.OrganizationID, Actor: p.Actor(),
			Action: audit.RestrictionDeleted, TargetKind: "restriction", TargetID: restrictionID, Detail: restrictionDetail(r),
		})
	})
}
