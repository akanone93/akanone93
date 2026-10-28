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
	"github.com/nightCode42/plux3/backend/internal/plxerr"
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
