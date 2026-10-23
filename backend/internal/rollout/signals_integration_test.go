// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/analytics/analyticstest"
	"github.com/nightCode42/plux3/backend/internal/rollout"
)

// writeEvents stores events of one release at the fixture's clock.
func writeEvents(t *testing.T, f *analyticstest.Fixture, release int64, events []analytics.Event) {
	t.Helper()
	b := analytics.Batch{
		OrganizationID: f.Org, AppID: f.App, EnvironmentID: f.Env, DeviceID: f.Device, DeviceClass: "phone", Locale: "de-DE",
	}
	for _, e := range events {
		id, err := f.IDs.New()
		if err != nil {
			t.Fatal(err)
		}
		e.ID, e.Time, e.ReleaseSequence, e.Category = id, f.Clock.Now().Add(-time.Hour), release, "analytics"
		b.Events = append(b.Events, e)
	}
	if err := f.Store.WriteEvents(context.Background(), b); err != nil {
		t.Fatal(err)
	}
}

// action is an action_run event with a result.
func action(result string) analytics.Event {
	return analytics.Event{Name: "action_run", Fields: fmt.Appendf(nil, `{"graph_id":"g","result":%q,"duration_ms":12}`, result)}
}

// call is a function_call event with a status.
func call(status string) analytics.Event {
	return analytics.Event{Name: "function_call", Fields: fmt.Appendf(nil, `{"function":"f","version":1,"status":%s,"duration_ms":30}`, status)}
}

// TestHealthGateSeesActionAndFunctionErrors derives the action_error and
// function_error series from the action_run and function_call events the
// rollups hold, per release, so that the health gate's error rates have
// their numerators: a cancelled run and a successful call are no failures,
// and another release's failures are not counted.
// Verifies: REL-053, ANL-010.
func TestHealthGateSeesActionAndFunctionErrors_REL_053(t *testing.T) {
	t.Parallel()
	f := analyticstest.New(t, analyticstest.Options{})
	writeEvents(t, f, 7, []analytics.Event{
		action("ok"), action("ok"), action("ok"), action("failed"), action("failed"), action("cancelled"),
		call(`"ok"`), call(`"ok"`), call(`"denied"`), call("503"), call("200"),
	})
	writeEvents(t, f, 8, []analytics.Event{action("failed"), call(`"timeout"`)})
	// The database stamps the receipt with its own clock, not the fixture's.
	if err := f.Store.Rollup(context.Background(), f.Org, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	signals := rollout.AnalyticsSignals{Store: f.Store}
	got, err := signals.Counts(context.Background(), f.Org, rollout.SignalQuery{
		AppID: f.App, EnvironmentID: f.Env, Release: 7, From: f.Clock.Now().Add(-3 * time.Hour), To: f.Clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Actions != 6 || got.ActionErrors != 2 || got.Calls != 5 || got.CallErrors != 2 {
		t.Errorf("actions %d (%d failed), calls %d (%d failed), want 6 (2), 5 (2)", got.Actions, got.ActionErrors, got.Calls, got.CallErrors)
	}
}
