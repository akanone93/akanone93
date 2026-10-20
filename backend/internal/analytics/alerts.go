// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// maxRules is the number of rules an organisation may define: as many as a
// page lists (api.pageSize), so that the list is always complete.
func (s *Service) maxRules() int64 { return s.o.Limits.Get(limits.APIPageSize) }

// CreateAlertRule defines an alert rule (ANL-030). It needs alerts.manage on
// the rule's app and is audited.
func (s *Service) CreateAlertRule(ctx context.Context, p auth.Principal, r Rule) (Rule, error) {
	if err := p.AuthorizeApp(auth.AlertsManage, r.AppID); err != nil {
		return Rule{}, fmt.Errorf("analytics: %w", err)
	}
	if err := r.Validate(); err != nil {
		return Rule{}, err
	}
	id, err := s.o.IDs.New()
	if err != nil {
		return Rule{}, fmt.Errorf("analytics: %w", err)
	}
	var out Rule
	err = s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		n, err := q.CountAlertRules(ctx, storage.MustUUID(p.OrganizationID))
		if err != nil {
			return fmt.Errorf("analytics: count the rules: %w", err)
		}
		if n >= s.maxRules() {
			return plxerr.New(plxerr.LimitExceeded, "an organisation has at most %d alert rules (api.pageSize)", s.maxRules())
		}
		params, err := s.ruleParams(ctx, q, p, id, r)
		if err != nil {
			return err
		}
		row, err := q.InsertAlertRule(ctx, params)
		if err != nil {
			return ruleFailure(err)
		}
		if out, err = ruleOf(row); err != nil {
			return err
		}
		out.Environment = r.Environment
		return s.auditRule(ctx, tx, p, audit.AlertRuleSet, out, "created")
	})
	return out, err
}

// ruleParams turns a validated rule into the row to insert.
func (s *Service) ruleParams(ctx context.Context, q *dbgen.Queries, p auth.Principal, id string, r Rule) (dbgen.InsertAlertRuleParams, error) {
	env, err := s.environmentID(ctx, q, r)
	if err != nil {
		return dbgen.InsertAlertRuleParams{}, err
	}
	filters, err := json.Marshal(r.Filters)
	if err != nil {
		return dbgen.InsertAlertRuleParams{}, fmt.Errorf("analytics: %w", err)
	}
	return dbgen.InsertAlertRuleParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(p.OrganizationID), AppID: storage.MustUUID(r.AppID),
		Name: r.Name, Metric: r.Metric, EnvironmentID: env, Filters: filters, Aggregation: string(r.Aggregation),
		WindowSeconds: int32(r.Window / time.Second), Comparison: string(r.Comparison), Threshold: r.Threshold, //nolint:gosec // G115: bounded by Validate.
		ForSeconds: int32(r.For / time.Second), Severity: string(r.Severity), Enabled: r.Enabled, //nolint:gosec // G115: bounded by Validate.
		CreatedBy: p.Actor().ID, CreatedAt: storage.Timestamp(s.now()),
	}, nil
}

// environmentID resolves the rule's environment key, if it names one.
func (s *Service) environmentID(ctx context.Context, q *dbgen.Queries, r Rule) (pgtype.UUID, error) {
	if r.Environment == "" {
		return pgtype.UUID{}, nil
	}
	env, err := q.GetEnvironmentByKey(ctx, dbgen.GetEnvironmentByKeyParams{AppID: storage.MustUUID(r.AppID), Key: r.Environment})
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, plxerr.New(plxerr.ResourceNotFound, "the app has no environment %q", r.Environment)
	}
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("analytics: environment: %w", err)
	}
	return env.ID, nil
}

// UpdateAlertRule replaces a rule's definition. The rule starts again from
// ok, because what it measured before no longer describes it. It needs
// alerts.manage on the rule's app and is audited.
func (s *Service) UpdateAlertRule(ctx context.Context, p auth.Principal, r Rule) (Rule, error) {
	if err := r.Validate(); err != nil {
		return Rule{}, err
	}
	id, err := storage.UUID(r.ID)
	if err != nil {
		return Rule{}, plxerr.New(plxerr.InvalidFormat, "the rule identifier is not valid")
	}
	var out Rule
	err = s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		cur, err := s.ownedRule(ctx, q, p, id, auth.AlertsManage)
		if err != nil {
			return err
		}
		r.AppID = storage.ID(cur.AppID)
		params, err := s.ruleParams(ctx, q, p, r.ID, r)
		if err != nil {
			return err
		}
		row, err := q.UpdateAlertRule(ctx, dbgen.UpdateAlertRuleParams{
			ID: id, OrganizationID: cur.OrganizationID, Name: params.Name, Metric: params.Metric, EnvironmentID: params.EnvironmentID,
			Filters: params.Filters, Aggregation: params.Aggregation, WindowSeconds: params.WindowSeconds, Comparison: params.Comparison,
			Threshold: params.Threshold, ForSeconds: params.ForSeconds, Severity: params.Severity, Enabled: params.Enabled,
			UpdatedBy: p.Actor().ID, UpdatedAt: storage.Timestamp(s.now()),
		})
		if err != nil {
			return ruleFailure(err)
		}
		if err := q.DeleteAlertState(ctx, id); err != nil {
			return fmt.Errorf("analytics: reset the state: %w", err)
		}
		if out, err = ruleOf(row); err != nil {
			return err
		}
		out.Environment = r.Environment
		return s.auditRule(ctx, tx, p, audit.AlertRuleSet, out, "changed")
	})
	return out, err
}

// DeleteAlertRule removes a rule and its state. It needs alerts.manage on
// the rule's app and is audited.
func (s *Service) DeleteAlertRule(ctx context.Context, p auth.Principal, ruleID string) error {
	id, err := storage.UUID(ruleID)
	if err != nil {
		return plxerr.New(plxerr.InvalidFormat, "the rule identifier is not valid")
	}
	return s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		cur, err := s.ownedRule(ctx, q, p, id, auth.AlertsManage)
		if err != nil {
			return err
		}
		if _, err := q.DeleteAlertRule(ctx, dbgen.DeleteAlertRuleParams{ID: id, OrganizationID: cur.OrganizationID}); err != nil {
			return fmt.Errorf("analytics: delete the rule: %w", err)
		}
		r, err := ruleOf(cur)
		if err != nil {
			return err
		}
		return s.auditRule(ctx, tx, p, audit.AlertRuleDeleted, r, "deleted")
	})
}

// ownedRule loads a rule of the principal's organisation and checks the
// permission on its app. A rule of another organisation is not found: row
// level security hides it.
func (s *Service) ownedRule(ctx context.Context, q *dbgen.Queries, p auth.Principal, id pgtype.UUID, want auth.Permission) (dbgen.AlertRule, error) {
	row, err := q.GetAlertRule(ctx, dbgen.GetAlertRuleParams{ID: id, OrganizationID: storage.MustUUID(p.OrganizationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.AlertRule{}, plxerr.New(plxerr.ResourceNotFound, "there is no such alert rule")
	}
	if err != nil {
		return dbgen.AlertRule{}, fmt.Errorf("analytics: load the rule: %w", err)
	}
	if err := p.AuthorizeApp(want, storage.ID(row.AppID)); err != nil {
		return dbgen.AlertRule{}, fmt.Errorf("analytics: %w", err)
	}
	return row, nil
}

// ListAlertRules lists the rules of an app, or, with no app, those of the
// organisation on apps the principal may read analytics of (ANL-030).
func (s *Service) ListAlertRules(ctx context.Context, p auth.Principal, appID string) ([]Rule, error) {
	app, err := s.readScope(p, appID)
	if err != nil {
		return nil, err
	}
	var out []Rule
	err = s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		rows, err := q.ListAlertRules(ctx, dbgen.ListAlertRulesParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), AppID: app, RowLimit: int32(s.maxRules()), //nolint:gosec // G115: a page size.
		})
		if err != nil {
			return fmt.Errorf("analytics: list the rules: %w", err)
		}
		for _, row := range rows {
			if !p.HoldsOnApp(auth.AnalyticsRead, storage.ID(row.AppID)) {
				continue
			}
			r, err := s.ruleWithEnvironment(ctx, q, row)
			if err != nil {
				return err
			}
			out = append(out, r)
		}
		return nil
	})
	return out, err
}

// ruleWithEnvironment converts a row and names its environment.
func (s *Service) ruleWithEnvironment(ctx context.Context, q *dbgen.Queries, row dbgen.AlertRule) (Rule, error) {
	r, err := ruleOf(row)
	if err != nil || !row.EnvironmentID.Valid {
		return r, err
	}
	env, err := q.GetEnvironment(ctx, row.EnvironmentID)
	if err != nil {
		return Rule{}, fmt.Errorf("analytics: environment: %w", err)
	}
	r.Environment = env.Key
	return r, nil
}

// readScope checks the right to read analytics, of one app or of the
// organisation, and returns the app to narrow a query to.
func (s *Service) readScope(p auth.Principal, appID string) (pgtype.UUID, error) {
	if appID == "" {
		return pgtype.UUID{}, p.Authorize(auth.AnalyticsRead) //nolint:wrapcheck // an authorisation error is final.
	}
	if err := p.AuthorizeApp(auth.AnalyticsRead, appID); err != nil {
		return pgtype.UUID{}, fmt.Errorf("analytics: %w", err)
	}
	app, err := storage.UUID(appID)
	if err != nil {
		return pgtype.UUID{}, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	return app, nil
}

// ListAlertStates lists where rules stand, optionally of one app and in one
// phase (ANL-030).
func (s *Service) ListAlertStates(ctx context.Context, p auth.Principal, appID string, phase AlertPhase) ([]AlertState, error) {
	app, err := s.readScope(p, appID)
	if err != nil {
		return nil, err
	}
	if phase != "" && !slices.Contains([]AlertPhase{PhaseOK, PhasePending, PhaseFiring, PhaseResolved}, phase) {
		return nil, plxerr.New(plxerr.InvalidEnumValue, "%q is not a phase; use ok, pending, firing or resolved", phase)
	}
	var out []AlertState
	err = s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListAlertStates(ctx, dbgen.ListAlertStatesParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), AppID: app, State: string(phase), RowLimit: int32(s.maxRules()), //nolint:gosec // G115: a page size.
		})
		if err != nil {
			return fmt.Errorf("analytics: list the states: %w", err)
		}
		for _, row := range rows {
			if p.HoldsOnApp(auth.AnalyticsRead, storage.ID(row.AppID)) {
				out = append(out, AlertState{
					RuleID: storage.ID(row.RuleID), AppID: storage.ID(row.AppID), RuleName: row.RuleName, Phase: AlertPhase(row.State),
					Since: storage.Time(row.Since), EvaluatedAt: storage.Time(row.EvaluatedAt), Value: row.Value,
					FiredAt: storage.Time(row.FiredAt), ResolvedAt: storage.Time(row.ResolvedAt),
				})
			}
		}
		return nil
	})
	return out, err
}

// auditRule records a change to a rule.
func (s *Service) auditRule(ctx context.Context, tx pgx.Tx, p auth.Principal, action audit.Action, r Rule, what string) error {
	_, err := s.o.Audit.Append(ctx, tx, audit.Entry{
		Action: action, TargetKind: "alert_rule", TargetID: r.ID, OrganizationID: p.OrganizationID, Actor: p.Actor(),
		Detail: fmt.Sprintf("%s %q: %s of %s over %s %s %g for %s, %s", what, r.Name, r.Aggregation, r.Metric, r.Window, r.Comparison, r.Threshold, r.For, r.Severity),
	})
	return err //nolint:wrapcheck // wrapped by the audit log
}

// ruleFailure maps a database failure on a rule to a user-facing one.
func ruleFailure(err error) error {
	var pg *pgconn.PgError
	switch {
	case errors.As(err, &pg) && pg.Code == "23505":
		return plxerr.New(plxerr.DuplicateKey, "the app already has an alert rule of that name")
	case errors.As(err, &pg) && pg.Code == "23503":
		return plxerr.New(plxerr.ResourceNotFound, "the app or environment does not exist")
	}
	return fmt.Errorf("analytics: save the rule: %w", err)
}

// ruleOf converts a row.
func ruleOf(row dbgen.AlertRule) (Rule, error) {
	r := Rule{
		ID: storage.ID(row.ID), AppID: storage.ID(row.AppID), Name: row.Name, Metric: row.Metric, EnvironmentID: storage.ID(row.EnvironmentID),
		Aggregation: Aggregation(row.Aggregation), Window: time.Duration(row.WindowSeconds) * time.Second, Comparison: Comparison(row.Comparison),
		Threshold: row.Threshold, For: time.Duration(row.ForSeconds) * time.Second, Severity: Severity(row.Severity), Enabled: row.Enabled,
		Revision: row.Revision, UpdatedBy: row.UpdatedBy, UpdatedAt: storage.Time(row.UpdatedAt),
	}
	if err := json.Unmarshal(row.Filters, &r.Filters); err != nil {
		return Rule{}, fmt.Errorf("analytics: the filters of rule %s: %w", r.ID, err)
	}
	return r, nil
}
