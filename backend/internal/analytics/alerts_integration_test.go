// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/analytics/analyticstest"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// published records the events a service announces.
type published struct {
	mu     sync.Mutex
	events []notify.Event
}

func (p *published) Publish(_ context.Context, e notify.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, e)
	return nil
}

func (p *published) states() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, e := range p.events {
		out = append(out, e.Attributes["state"])
	}
	return out
}

// evaluator builds another service on the fixture's database that tells p,
// as a second worker would.
func evaluator(t *testing.T, f *analyticstest.Fixture, p *published) *analytics.Service {
	t.Helper()
	window, err := analytics.ParseWindow("02:00", "04:00", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := analytics.New(analytics.Options{
		Window: window, DB: f.DB, Store: f.Store, Audit: f.Audit, IDs: f.IDs, Limits: f.Limits, Now: f.Clock.Now, Notify: p,
		Defaults:      analytics.Defaults{RawDays: 396, AuditDays: 3653},
		Organizations: func(ctx context.Context, fn func(org string) error) error { return fn(f.Org) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func crashRule(f *analyticstest.Fixture, name string, hold time.Duration) analytics.Rule {
	return analytics.Rule{
		AppID: f.App, Name: name, Metric: "screen_view", Aggregation: analytics.AggCount, Window: 30 * time.Minute,
		Comparison: analytics.Greater, Threshold: 2, For: hold, Severity: analytics.SeverityCritical, Enabled: true,
	}
}

func phaseOf(t *testing.T, svc *analytics.Service, f *analyticstest.Fixture, rule string) analytics.AlertPhase {
	t.Helper()
	states, err := svc.ListAlertStates(context.Background(), f.Owner, f.App, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.RuleID == rule {
			return s.Phase
		}
	}
	t.Fatalf("rule %s has no state", rule)
	return ""
}

// Verifies: ANL-030.
// A rule fires once its metric breaches, notifies once however many
// evaluators run, and resolves with a second notification.
func TestEvaluateAlerts_ANL_030(t *testing.T) {
	f := analyticstest.New(t, analyticstest.Options{})
	ctx := context.Background()
	told := &published{}
	one, two := evaluator(t, f, told), evaluator(t, f, told)
	rule, err := one.CreateAlertRule(ctx, f.Owner, crashRule(f, "busy", 0))
	if err != nil {
		t.Fatal(err)
	}
	base := f.Clock.Now().Truncate(time.Hour)
	write(t, f, []sample{{at: base.Add(-3 * time.Minute), page: "/a", value: 1}, {at: base.Add(-2 * time.Minute), page: "/a", value: 1}, {at: base.Add(-time.Minute), page: "/a", value: 1}})
	rollUp(t, f, time.Now().Add(time.Hour))

	f.Clock.Set(base.Add(3 * time.Minute))
	for _, svc := range []*analytics.Service{one, two, one} {
		if err := svc.EvaluateAlerts(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := told.states(); len(got) != 1 || got[0] != "firing" {
		t.Fatalf("notices after three evaluations = %v", got)
	}
	if told.events[0].Type != notify.AlertFired || told.events[0].AppID != f.App || told.events[0].Attributes["severity"] != "critical" {
		t.Errorf("event = %+v", told.events[0])
	}
	if p := phaseOf(t, one, f, rule.ID); p != analytics.PhaseFiring {
		t.Errorf("phase = %s", p)
	}

	f.Clock.Set(base.Add(time.Hour))
	if err := two.EvaluateAlerts(ctx); err != nil {
		t.Fatal(err)
	}
	if got := told.states(); len(got) != 2 || got[1] != "resolved" {
		t.Errorf("notices after the window emptied = %v", got)
	}
	if p := phaseOf(t, one, f, rule.ID); p != analytics.PhaseResolved {
		t.Errorf("phase = %s", p)
	}
}

// Verifies: ANL-030.
// A rule with a for-duration stays pending until the breach has held that
// long, and a disabled or deleted rule is not evaluated.
func TestEvaluateAlertsFor_ANL_030(t *testing.T) {
	f := analyticstest.New(t, analyticstest.Options{})
	ctx := context.Background()
	told := &published{}
	svc := evaluator(t, f, told)
	held, err := svc.CreateAlertRule(ctx, f.Owner, crashRule(f, "held", 2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	off := crashRule(f, "off", 0)
	off.Enabled = false
	if _, err := svc.CreateAlertRule(ctx, f.Owner, off); err != nil {
		t.Fatal(err)
	}
	base := f.Clock.Now().Truncate(time.Hour)
	write(t, f, []sample{{at: base.Add(-3 * time.Minute), value: 1}, {at: base.Add(-2 * time.Minute), value: 1}, {at: base.Add(-time.Minute), value: 1}})
	rollUp(t, f, time.Now().Add(time.Hour))

	for i, want := range []analytics.AlertPhase{analytics.PhasePending, analytics.PhasePending, analytics.PhaseFiring} {
		f.Clock.Set(base.Add(time.Duration(3+i) * time.Minute))
		if err := svc.EvaluateAlerts(ctx); err != nil {
			t.Fatal(err)
		}
		if got := phaseOf(t, svc, f, held.ID); got != want {
			t.Errorf("minute %d: phase = %s, want %s", 3+i, got, want)
		}
	}
	if got := told.states(); len(got) != 1 {
		t.Errorf("notices = %v, want one", got)
	}
	states, err := svc.ListAlertStates(ctx, f.Owner, f.App, analytics.PhaseFiring)
	if err != nil || len(states) != 1 {
		t.Errorf("firing states = %d, %v", len(states), err)
	}
}

// Verifies: ANL-030, SEC-102.
// Rules are validated, limited to those who manage alerts, and changed
// rules start from ok.
func TestAlertRuleChanges_ANL_030(t *testing.T) {
	f := analyticstest.New(t, analyticstest.Options{})
	ctx := context.Background()
	svc := f.Analytics
	if _, err := svc.CreateAlertRule(ctx, f.Viewer(), crashRule(f, "denied", 0)); err == nil {
		t.Error("a viewer created a rule")
	}
	bad := crashRule(f, "bad", 0)
	bad.Comparison = "!="
	if _, err := svc.CreateAlertRule(ctx, f.Owner, bad); !errors.Is(err, plxerr.Sentinel(plxerr.InvalidEnumValue)) {
		t.Errorf("a bad comparison: %v", err)
	}
	rule, err := svc.CreateAlertRule(ctx, f.Owner, crashRule(f, "ok", 0))
	if err != nil {
		t.Fatal(err)
	}
	rule.Threshold = 10
	updated, err := svc.UpdateAlertRule(ctx, f.Owner, rule)
	if err != nil || updated.Revision <= rule.Revision || updated.Threshold != 10 {
		t.Errorf("update = %+v, %v", updated, err)
	}
	if _, err := svc.CreateAlertRule(ctx, f.Owner, crashRule(f, "ok", 0)); err == nil {
		t.Error("a second rule with the same name was created")
	}
	viewer := f.Viewer()
	if rules, err := svc.ListAlertRules(ctx, viewer, f.App); err != nil || len(rules) != 1 {
		t.Errorf("a viewer lists %d rules, %v", len(rules), err)
	}
	if err := svc.DeleteAlertRule(ctx, auth.Principal{OrganizationID: f.Org}, rule.ID); err == nil {
		t.Error("a caller without permissions deleted a rule")
	}
	if err := svc.DeleteAlertRule(ctx, f.Owner, rule.ID); err != nil {
		t.Fatal(err)
	}
	if rules, _ := svc.ListAlertRules(ctx, f.Owner, f.App); len(rules) != 0 {
		t.Errorf("%d rules after the delete", len(rules))
	}
}
