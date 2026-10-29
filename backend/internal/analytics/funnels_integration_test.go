// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/analytics/analyticstest"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

func TestFunnels_ANL_020(t *testing.T) {
	f := analyticstest.New(t, analyticstest.Options{})
	ctx := context.Background()
	svc := f.Analytics
	fn := analytics.Funnel{AppID: f.App, Name: "checkout", Steps: []string{"screen_view", "action_run"}}
	if _, err := svc.CreateFunnel(ctx, f.Viewer(), fn); err == nil {
		t.Error("a viewer created a funnel")
	}
	short := fn
	short.Steps = []string{"screen_view"}
	if _, err := svc.CreateFunnel(ctx, f.Owner, short); !errors.Is(err, plxerr.Sentinel(plxerr.OutOfRange)) {
		t.Errorf("a one-step funnel: %v", err)
	}
	created, err := svc.CreateFunnel(ctx, f.Owner, fn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateFunnel(ctx, f.Owner, fn); !errors.Is(err, plxerr.Sentinel(plxerr.DuplicateKey)) {
		t.Errorf("a second funnel of the same name: %v", err)
	}
	if list, err := svc.ListFunnels(ctx, f.Viewer(), f.App); err != nil || len(list) != 1 {
		t.Errorf("a viewer lists %d funnels, %v", len(list), err)
	}
	now := f.Clock.Now()
	got, steps, err := svc.FunnelResult(ctx, f.Viewer(), created.ID, now.Add(-48*time.Hour), now.Add(time.Hour))
	if err != nil || got.ID != created.ID || len(steps) != 2 || steps[0].Event != "screen_view" {
		t.Errorf("result = %+v, %+v, %v", got, steps, err)
	}
	if apps, err := svc.Overview(ctx, f.Viewer(), now.Add(-48*time.Hour), now.Add(time.Hour)); err != nil || len(apps) != 1 || apps[0].AppID != f.App {
		t.Errorf("overview = %+v, %v", apps, err)
	}
	other := f.Owner
	other.OrganizationID = "018f0000-0000-7000-8000-00000000f00d"
	if _, _, err := svc.FunnelResult(ctx, other, created.ID, now.Add(-time.Hour), now); !errors.Is(err, plxerr.Sentinel(plxerr.ResourceNotFound)) {
		t.Errorf("another organisation read the funnel: %v", err)
	}
	if err := svc.DeleteFunnel(ctx, other, created.ID); !errors.Is(err, plxerr.Sentinel(plxerr.ResourceNotFound)) {
		t.Errorf("another organisation deleted the funnel: %v", err)
	}
	if err := svc.DeleteFunnel(ctx, f.Owner, created.ID); err != nil {
		t.Fatal(err)
	}
}

// funnelEvents writes the events of one device, a second apart from start.
func funnelEvents(t *testing.T, f *analyticstest.Fixture, device string, start time.Time, names ...string) {
	t.Helper()
	b := analytics.Batch{OrganizationID: f.Org, AppID: f.App, EnvironmentID: f.Env, DeviceID: device}
	for i, name := range names {
		id, err := f.IDs.New()
		if err != nil {
			t.Fatal(err)
		}
		b.Events = append(b.Events, analytics.Event{ID: id, Name: name, Time: start.Add(time.Duration(i) * time.Second), Category: "necessary"})
	}
	if err := f.Store.WriteEvents(context.Background(), b); err != nil {
		t.Fatal(err)
	}
}

func TestFunnelResultCountsOrderedSessions_ANL_020(t *testing.T) {
	f := analyticstest.New(t, analyticstest.Options{})
	ctx := context.Background()
	fn, err := f.Analytics.CreateFunnel(ctx, f.Owner, analytics.Funnel{AppID: f.App, Name: "checkout", Steps: []string{"screen_view", "action_run"}})
	if err != nil {
		t.Fatal(err)
	}
	start := f.Clock.Now().Add(-time.Hour)
	other, _ := f.NewDevice(t)
	third, _ := f.NewDevice(t)
	// The device converts in its first session and only reaches step one in its second.
	funnelEvents(t, f, f.Device, start, "session_start", "screen_view", "action_run", "session_start", "screen_view", "screen_view")
	// Out of order: the action came before the screen.
	funnelEvents(t, f, other, start, "session_start", "action_run", "screen_view")
	// Across sessions: step one in the first, step two in the second.
	funnelEvents(t, f, third, start, "session_start", "screen_view", "session_start", "action_run")
	// Outside the range.
	funnelEvents(t, f, third, start.Add(-48*time.Hour), "session_start", "screen_view", "action_run")
	_, steps, err := f.Analytics.FunnelResult(ctx, f.Viewer(), fn.ID, start.Add(-time.Minute), start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[0].Sessions != 4 || steps[1].Sessions != 1 || steps[1].Events != 1 || steps[0].Conversion != 1 || steps[1].Conversion != 0.25 {
		t.Errorf("steps = %+v, want 4 then 1 sessions", steps)
	}
	noRead := auth.Principal{Identity: f.Owner.Identity, OrganizationID: f.Org, Permissions: []auth.Permission{auth.AppRead}}
	if _, _, err := f.Analytics.FunnelResult(ctx, noRead, fn.ID, start, start.Add(time.Hour)); err == nil {
		t.Error("a principal without analytics.read read the result")
	}
}

func TestFunnelResultScanLimit_ANL_020(t *testing.T) {
	lim, err := limits.Defaults().Tighten(limits.AnalyticsFunnelScanEvents, limits.ScopeOrganization, 3)
	if err != nil {
		t.Fatal(err)
	}
	f := analyticstest.New(t, analyticstest.Options{Limits: lim})
	ctx := context.Background()
	fn, err := f.Analytics.CreateFunnel(ctx, f.Owner, analytics.Funnel{AppID: f.App, Name: "checkout", Steps: []string{"screen_view", "action_run"}})
	if err != nil {
		t.Fatal(err)
	}
	start := f.Clock.Now().Add(-time.Hour)
	funnelEvents(t, f, f.Device, start, "session_start", "screen_view", "action_run")
	if _, _, err := f.Analytics.FunnelResult(ctx, f.Owner, fn.ID, start.Add(-time.Minute), start.Add(time.Hour)); err != nil {
		t.Errorf("a range at the limit: %v", err)
	}
	funnelEvents(t, f, f.Device, start.Add(time.Minute), "screen_view")
	if _, _, err := f.Analytics.FunnelResult(ctx, f.Owner, fn.ID, start.Add(-time.Minute), start.Add(time.Hour)); !errors.Is(err, plxerr.Sentinel(plxerr.LimitExceeded)) {
		t.Errorf("a range over the limit: %v", err)
	}
}
