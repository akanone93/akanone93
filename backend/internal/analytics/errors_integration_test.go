// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/analytics/analyticstest"
)

// reportError records one error event of a device.
func reportError(t *testing.T, f *analyticstest.Fixture, device, id, code, node, stack string) {
	t.Helper()
	fields := fmt.Sprintf(`{"code":%q,"node_path":%q,"frames":%q}`, code, node, stack)
	b := analytics.Batch{OrganizationID: f.Org, AppID: f.App, EnvironmentID: f.Env, DeviceID: device, Events: []analytics.Event{{
		ID: id, Name: analytics.EventError, Time: f.Clock.Now(), ReleaseSequence: 3, PluginKey: "main", Route: "/loans", Fields: []byte(fields),
	}}}
	if err := f.Analytics.RecordErrors(context.Background(), b); err != nil {
		t.Fatal(err)
	}
}

// Verifies: ANL-040.
// The same error from two devices is one group, a different path another;
// an event delivered twice is counted once; the group keeps its devices,
// trend and samples.
func TestErrorGroups_ANL_040(t *testing.T) {
	f := analyticstest.New(t, analyticstest.Options{})
	ctx := context.Background()
	other, _ := f.NewDevice(t)
	const stackA = "#0  Foo.bar (package:app/src/x.dart:12:5)\n#1  Baz.qux (package:app/y.dart:7:1)"
	const stackB = "#0   Foo.bar (package:app/src/x.dart:40:9)\n#1  Baz.qux (package:app/y.dart:70:3)"
	const first, second, third, fourth = "018f0000-0000-7000-8000-000000000001", "018f0000-0000-7000-8000-000000000002",
		"018f0000-0000-7000-8000-000000000003", "018f0000-0000-7000-8000-000000000004"

	reportError(t, f, f.Device, first, "PLX-4101", "root/0", stackA)
	reportError(t, f, other, second, "PLX-4101", "root/0", stackB)
	reportError(t, f, other, second, "PLX-4101", "root/0", stackB)
	reportError(t, f, f.Device, third, "PLX-4101", "root/1", stackA)
	reportError(t, f, f.Device, fourth, "PLX-4101", "", "")

	groups, err := f.Analytics.ListErrorGroups(ctx, f.Viewer(), f.App, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 3 {
		t.Fatalf("%d groups, want 3: %+v", len(groups), groups)
	}
	var shared analytics.ErrorGroup
	for _, g := range groups {
		if g.NodePath == "root/0" {
			shared = g
		}
	}
	if shared.Events != 2 || shared.Devices != 2 || shared.ReleaseSequence != 3 || len(shared.Frames) != 2 {
		t.Errorf("shared group = %+v", shared)
	}

	g, samples, err := f.Analytics.GetErrorGroup(ctx, f.Viewer(), shared.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 || len(g.Trend) != 1 || g.Trend[0].Events != 2 || g.JSONPath != "" {
		t.Errorf("group = %+v, %d samples", g, len(samples))
	}
	if released, _ := f.Analytics.ListErrorGroups(ctx, f.Viewer(), f.App, 4, 0, 0); len(released) != 0 {
		t.Errorf("release 4 has %d groups", len(released))
	}
}
