// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// AlertEvery is how often the worker evaluates alert rules.
const AlertEvery = time.Minute

// EvaluateAlerts evaluates every enabled rule of every organisation against
// the rollups (ANL-030). The evaluation window is the current minute, so a
// repeated call within the minute, a retried job or a second worker changes
// nothing: each rule's state records the window it last applied. A rule
// that cannot be evaluated does not stop the others; the errors are joined.
func (s *Service) EvaluateAlerts(ctx context.Context) error {
	at := s.now().UTC().Truncate(AlertEvery)
	var errs []error
	err := s.o.Organizations(ctx, func(org string) error {
		if err := s.evaluateOrg(ctx, org, at); err != nil {
			errs = append(errs, err)
		}
		return nil
	})
	return errors.Join(append(errs, err)...)
}

// evaluateOrg evaluates an organisation's enabled rules.
func (s *Service) evaluateOrg(ctx context.Context, org string, at time.Time) error {
	var rows []dbgen.AlertRule
	err := s.inOrg(ctx, org, "", func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = dbgen.New(tx).ListEnabledAlertRules(ctx, storage.MustUUID(org))
		return err //nolint:wrapcheck // wrapped below
	})
	if err != nil {
		return fmt.Errorf("analytics: list the rules of %s: %w", org, err)
	}
	var errs []error
	for _, row := range rows {
		if err := s.evaluateRule(ctx, org, row, at); err != nil {
			errs = append(errs, fmt.Errorf("rule %s: %w", storage.ID(row.ID), err))
		}
	}
	return errors.Join(errs...)
}

// evaluateRule measures one rule and applies the result to its state.
func (s *Service) evaluateRule(ctx context.Context, org string, row dbgen.AlertRule, at time.Time) error {
	rule, err := ruleOf(row)
	if err != nil {
		return err
	}
	value, ok, err := s.measure(ctx, org, rule, at)
	if err != nil {
		return err
	}
	var v *float64
	if ok {
		v = &value
	}
	return s.inOrg(ctx, org, "", func(ctx context.Context, tx pgx.Tx) error {
		return s.apply(ctx, tx, org, rule, v, at)
	})
}

// measure aggregates the rollups of the rule's window, which ends at the
// last whole bucket before at. ok is false when there is no value.
func (s *Service) measure(ctx context.Context, org string, r Rule, at time.Time) (float64, bool, error) {
	g := r.granularity()
	to := at.Truncate(g.Width())
	q := Query{
		AppID: r.AppID, EnvironmentID: r.EnvironmentID, Metric: r.Metric, Granularity: g,
		From: to.Add(-r.Window), To: to, GroupBy: r.filterDimensions(), Limit: int(s.o.Limits.Get(limits.APIPageSize)),
	}
	var points []Point
	for {
		series, err := s.o.Store.Query(ctx, org, q)
		if err != nil {
			return 0, false, fmt.Errorf("analytics: query the rollups: %w", err)
		}
		for _, p := range series.Points {
			if r.matches(p) {
				points = append(points, p)
			}
		}
		if !series.Truncated {
			break
		}
		if !series.NextFrom.After(q.From) {
			return 0, false, errors.New("analytics: a rollup query did not advance")
		}
		q.From = series.NextFrom
	}
	v, ok := aggregate(r.Aggregation, points)
	return v, ok, nil
}

// filterDimensions are the dimensions a rule's query keeps apart, so that
// its filters can be matched against each point.
func (r Rule) filterDimensions() []Dimension {
	var out []Dimension
	for _, d := range Dimensions() {
		if _, ok := r.Filters[d]; ok {
			out = append(out, d)
		}
	}
	return out
}

// apply steps a rule's state in a transaction, announcing a transition
// before the state is committed: a notification that cannot be queued
// leaves the state as it was, so the next evaluation tries again, and a
// state that cannot be committed after a notification may announce the
// transition twice, which is the lesser failure. The state row is locked, so
// two evaluators of one rule take turns, and the second finds the window
// already applied.
func (s *Service) apply(ctx context.Context, tx pgx.Tx, org string, rule Rule, value *float64, at time.Time) error {
	q := dbgen.New(tx)
	id := storage.MustUUID(rule.ID)
	cur, err := q.GetAlertRule(ctx, dbgen.GetAlertRuleParams{ID: id, OrganizationID: storage.MustUUID(org)})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (cur.Revision != rule.Revision || !cur.Enabled)) {
		return nil // deleted, changed or disabled since it was measured
	}
	if err != nil {
		return fmt.Errorf("analytics: reload the rule: %w", err)
	}
	if err := q.EnsureAlertState(ctx, dbgen.EnsureAlertStateParams{RuleID: id, OrganizationID: cur.OrganizationID, Since: storage.Timestamp(at)}); err != nil {
		return fmt.Errorf("analytics: ensure the state: %w", err)
	}
	row, err := q.LockAlertState(ctx, id)
	if err != nil {
		return fmt.Errorf("analytics: lock the state: %w", err)
	}
	prev := AlertState{
		RuleID: rule.ID, Phase: AlertPhase(row.State), Since: storage.Time(row.Since), EvaluatedAt: storage.Time(row.EvaluatedAt),
		Value: row.Value, FiredAt: storage.Time(row.FiredAt), ResolvedAt: storage.Time(row.ResolvedAt),
	}
	breach := value != nil && rule.Comparison.holds(*value, rule.Threshold)
	next, notice := Step(prev, breach, value, at, rule.For)
	if next.EvaluatedAt.Equal(prev.EvaluatedAt) {
		return nil
	}
	if err := s.announce(ctx, org, rule, next, notice); err != nil {
		return err
	}
	if err := q.SetAlertState(ctx, dbgen.SetAlertStateParams{
		RuleID: id, State: string(next.Phase), Since: storage.Timestamp(next.Since), EvaluatedAt: storage.Timestamp(next.EvaluatedAt),
		Value: next.Value, FiredAt: storage.Timestamp(next.FiredAt), ResolvedAt: storage.Timestamp(next.ResolvedAt),
	}); err != nil {
		return fmt.Errorf("analytics: store the alert state: %w", err)
	}
	return nil
}

// announce publishes a firing or resolved notification. The recipients are
// whoever subscribed to alert.fired (ADR-0081): users and teams, in-app or
// through a channel, and webhook endpoints. The severity travels in the
// attributes, where a receiver can route on it.
func (s *Service) announce(ctx context.Context, org string, r Rule, st AlertState, n Notice) error {
	if n == NoNotice || s.o.Notify == nil {
		return nil
	}
	state, verb := "firing", "firing"
	if n == NoticeResolved {
		state, verb = "resolved", "resolved"
	}
	attrs := map[string]string{
		"rule": r.ID, "rule_name": r.Name, "state": state, "severity": string(r.Severity), "metric": r.Metric,
		"aggregation": string(r.Aggregation), "comparison": string(r.Comparison),
		"threshold": strconv.FormatFloat(r.Threshold, 'g', -1, 64), "window": r.Window.String(),
	}
	body := fmt.Sprintf("%s of %s over %s is %s", r.Aggregation, r.Metric, r.Window, describeValue(st.Value))
	if n == NoticeFired {
		body += fmt.Sprintf(", which is %s %g", r.Comparison, r.Threshold)
	}
	if st.Value != nil {
		attrs["value"] = strconv.FormatFloat(*st.Value, 'g', -1, 64)
	}
	err := s.o.Notify.Publish(ctx, notify.Event{
		Type: notify.AlertFired, OrganizationID: org, AppID: r.AppID, At: st.EvaluatedAt,
		Title: fmt.Sprintf("Alert %s: %s", verb, r.Name), Body: body, Attributes: attrs,
	})
	if err != nil {
		return fmt.Errorf("analytics: announce the alert: %w", err)
	}
	return nil
}

// describeValue renders an aggregated value for a notification.
func describeValue(v *float64) string {
	if v == nil {
		return "unknown"
	}
	return strconv.FormatFloat(*v, 'g', 6, 64)
}
