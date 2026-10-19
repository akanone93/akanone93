// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/analytics/analyticstest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

func code(err error) plxerr.Code {
	c, _ := plxerr.CodeOf(err)
	return c
}

func settingOf(t *testing.T, f *analyticstest.Fixture, class string) analytics.Setting {
	t.Helper()
	all, err := f.Analytics.GetRetention(context.Background(), f.Owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range all {
		if s.Class == class {
			return s
		}
	}
	t.Fatalf("no setting for %s", class)
	return analytics.Setting{}
}

func auditActions(t *testing.T, f *analyticstest.Fixture) []string {
	t.Helper()
	var out []string
	err := f.DB.InTx(context.Background(), storage.Tenant{OrganizationID: f.Org}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT action FROM audit_log WHERE action LIKE 'analytics.%' ORDER BY sequence")
		if err != nil {
			return err //nolint:wrapcheck // a test
		}
		defer rows.Close()
		for rows.Next() {
			var a string
			if err := rows.Scan(&a); err != nil {
				return err //nolint:wrapcheck // a test
			}
			out = append(out, a)
		}
		return rows.Err() //nolint:wrapcheck // a test
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Verifies: ANL-010, SEC-102.
// Retention is changed only by someone holding retention.manage with a
// second factor, and only to values the service accepts.
func TestRetentionAuthorisationAndValidation_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{})
	if _, _, err := f.Analytics.SetRetention(ctx, f.Viewer(), analytics.ClassRawEvents, 30, false); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer changes retention: %v", err)
	}
	if _, err := f.Analytics.GetRetention(ctx, f.Viewer()); err != nil {
		t.Errorf("a viewer reads retention: %v", err)
	}
	noFactor := f.Owner
	noFactor.SecondFactor = false
	if _, _, err := f.Analytics.SetRetention(ctx, noFactor, analytics.ClassRawEvents, 30, false); code(err) != plxerr.MultiFactorRequired {
		t.Errorf("without a second factor: %v", err)
	}
	for name, c := range map[string]struct {
		class string
		days  int
	}{"unknown class": {"logs", 30}, "negative": {analytics.ClassRawEvents, -1}, "too long": {analytics.ClassRawEvents, 36501}} {
		if _, _, err := f.Analytics.SetRetention(ctx, f.Owner, c.class, c.days, false); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if got := auditActions(t, f); len(got) != 0 {
		t.Errorf("refused changes were audited as made: %v", got)
	}
}

// Verifies: ANL-010.
// The defaults are 13 months of raw events, rollups for ever and ten years
// of production audit, which is recorded but not enforced.
func TestRetentionDefaults_ANL_010(t *testing.T) {
	t.Parallel()
	f := analyticstest.New(t, analyticstest.Options{})
	raw, rollups, audit := settingOf(t, f, analytics.ClassRawEvents), settingOf(t, f, analytics.ClassRollups), settingOf(t, f, analytics.ClassAuditProduction)
	if raw.Days == nil || *raw.Days != 396 || !raw.Default || !raw.Enforced {
		t.Errorf("raw events: %+v", raw)
	}
	if rollups.Days != nil || !rollups.Default || !rollups.Enforced {
		t.Errorf("rollups: %+v", rollups)
	}
	if audit.Days == nil || *audit.Days != 3653 || audit.Enforced {
		t.Errorf("production audit: %+v", audit)
	}
}

// Verifies: ANL-010, GOV-020.
// A change waits for the maintenance window and reports PLX-8301; it is
// listed while it waits, applies when the window opens, and is audited at
// every step. A newer request for the class replaces a waiting one.
func TestRetentionChangeWaitsForTheWindow_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{})
	first, diags, err := f.Analytics.SetRetention(ctx, f.Owner, analytics.ClassRawEvents, 90, false)
	if err != nil || first.State != "scheduled" || len(diags) != 1 || diags[0].Code != plxerr.RetentionChangeScheduled {
		t.Fatalf("SetRetention: %+v %v %v", first, diags, err)
	}
	if want := time.Date(2026, 10, 20, 2, 0, 0, 0, time.UTC); !first.ApplyAt.Equal(want) {
		t.Errorf("apply at %s, want %s", first.ApplyAt, want)
	}
	if s := settingOf(t, f, analytics.ClassRawEvents); !s.Default || *s.Days != 396 {
		t.Errorf("the change took effect before the window: %+v", s)
	}
	if err := f.Analytics.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	if s := settingOf(t, f, analytics.ClassRawEvents); !s.Default {
		t.Errorf("the change took effect outside the window: %+v", s)
	}

	second, _, err := f.Analytics.SetRetention(ctx, f.Owner, analytics.ClassRawEvents, 60, false)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := f.Analytics.ListChanges(ctx, f.Viewer(), true, 0)
	if err != nil || len(waiting) != 1 || waiting[0].ID != second.ID {
		t.Fatalf("waiting changes: %+v %v", waiting, err)
	}
	all, _ := f.Analytics.ListChanges(ctx, f.Owner, false, 0)
	if len(all) != 2 || all[1].ID != first.ID || all[1].State != "cancelled" {
		t.Errorf("the replaced change: %+v", all)
	}

	f.Clock.Set(time.Date(2026, 10, 20, 1, 59, 0, 0, time.UTC))
	if err := f.Analytics.Maintain(ctx); err != nil || !settingOf(t, f, analytics.ClassRawEvents).Default {
		t.Fatalf("a minute before the window: %v", err)
	}
	f.Clock.Set(time.Date(2026, 10, 20, 2, 5, 0, 0, time.UTC))
	if err := f.Analytics.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	if s := settingOf(t, f, analytics.ClassRawEvents); s.Default || *s.Days != 60 {
		t.Errorf("in the window: %+v", s)
	}
	if waiting, _ := f.Analytics.ListChanges(ctx, f.Owner, true, 0); len(waiting) != 0 {
		t.Errorf("an applied change is still waiting: %+v", waiting)
	}
	want := []string{"analytics.retention_scheduled", "analytics.retention_cancelled", "analytics.retention_scheduled", "analytics.retention_applied"}
	if got := auditActions(t, f); !slices.Equal(got, want) {
		t.Errorf("audit = %v, want %v", got, want)
	}
}

// Verifies: ANL-010.
// A scheduled change can be cancelled once, only by someone who may manage
// retention, and then never applies.
func TestRetentionCancel_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{})
	ch, _, err := f.Analytics.SetRetention(ctx, f.Owner, analytics.ClassRollups, 365, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Analytics.CancelChange(ctx, f.Viewer(), ch.ID); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer cancels: %v", err)
	}
	if _, err := f.Analytics.CancelChange(ctx, f.Owner, "nope"); code(err) != plxerr.InvalidFormat {
		t.Errorf("a malformed id: %v", err)
	}
	if c, err := f.Analytics.CancelChange(ctx, f.Owner, ch.ID); err != nil || c.State != "cancelled" {
		t.Fatalf("CancelChange: %+v %v", c, err)
	}
	if _, err := f.Analytics.CancelChange(ctx, f.Owner, ch.ID); code(err) != plxerr.ResourceNotFound {
		t.Errorf("a second cancel: %v", err)
	}
	f.Clock.Set(time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC))
	if err := f.Analytics.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	if s := settingOf(t, f, analytics.ClassRollups); !s.Default || s.Days != nil {
		t.Errorf("a cancelled change was applied: %+v", s)
	}
	if got, want := auditActions(t, f), []string{"analytics.retention_scheduled", "analytics.retention_cancelled"}; !slices.Equal(got, want) {
		t.Errorf("audit = %v, want %v", got, want)
	}
}

// Verifies: ANL-010.
// An immediate change takes effect and deletes what is past the new
// retention at once, keeping the rollups, which have their own retention.
func TestRetentionImmediate_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{})
	now := f.Clock.Now()
	write(t, f, []sample{
		{at: now.AddDate(0, 0, -100), value: 10}, {at: now.AddDate(0, 0, -10), value: 20}, {at: now.Add(-time.Hour), value: 30},
	})
	rollUp(t, f, time.Now().Add(time.Hour))
	ch, diags, err := f.Analytics.SetRetention(ctx, f.Owner, analytics.ClassRawEvents, 30, true)
	if err != nil || ch.State != "applied" || len(diags) != 0 {
		t.Fatalf("SetRetention: %+v %v %v", ch, diags, err)
	}
	if s := settingOf(t, f, analytics.ClassRawEvents); s.Default || *s.Days != 30 {
		t.Errorf("setting: %+v", s)
	}
	if got := f.Count(t, ""); got != 2 {
		t.Errorf("%d raw events remain, want the two within 30 days", got)
	}
	series := queryAll(t, f, analytics.Query{AppID: f.App, Metric: "screen_view", Granularity: analytics.Day, From: now.AddDate(0, 0, -120), To: now.Add(time.Hour)})
	var events int64
	for _, p := range series.Points {
		events += p.Events
	}
	if events != 3 {
		t.Errorf("the rollups count %d events, want all 3: they are kept", events)
	}

	if _, _, err := f.Analytics.SetRetention(ctx, f.Owner, analytics.ClassRollups, 50, true); err != nil {
		t.Fatal(err)
	}
	series = queryAll(t, f, analytics.Query{AppID: f.App, Metric: "screen_view", Granularity: analytics.Day, From: now.AddDate(0, 0, -120), To: now.Add(time.Hour)})
	events = 0
	for _, p := range series.Points {
		events += p.Events
	}
	if events != 2 {
		t.Errorf("after rollups retention of 50 days, %d events remain, want 2", events)
	}
	if got, want := auditActions(t, f), []string{"analytics.retention_set", "analytics.retention_set"}; !slices.Equal(got, want) {
		t.Errorf("audit = %v, want %v", got, want)
	}
}

// Verifies: ANL-010.
// A partition no organisation needs any more is dropped, and the production
// audit retention is recorded without deleting a single audit entry.
func TestMaintainDropsOldPartitionsNotAudit_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{})
	now := f.Clock.Now()
	old := now.AddDate(0, 0, -500)
	write(t, f, []sample{{at: old, value: 1}, {at: now, value: 1}})
	oldPartition := "analytics_events_" + old.Format("200601")
	if !slices.Contains(f.Partitions(t), oldPartition) {
		t.Fatalf("no partition %s in %v", oldPartition, f.Partitions(t))
	}
	if _, _, err := f.Analytics.SetRetention(ctx, f.Owner, analytics.ClassAuditProduction, 30, true); err != nil {
		t.Fatal(err)
	}
	var before, after int
	count := func(n *int) {
		err := f.DB.InTx(ctx, storage.Tenant{OrganizationID: f.Org}, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM audit_log").Scan(n)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	count(&before)
	if err := f.Analytics.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	count(&after)
	if slices.Contains(f.Partitions(t), oldPartition) {
		t.Errorf("the partition %s outlived the retention of every organisation", oldPartition)
	}
	if after < before || f.Count(t, "") != 1 {
		t.Errorf("audit entries %d -> %d, raw events %d; want audit kept and one event", before, after, f.Count(t, ""))
	}
	if s := settingOf(t, f, analytics.ClassAuditProduction); *s.Days != 30 || s.Enforced {
		t.Errorf("audit setting: %+v", s)
	}
}

// Verifies: ANL-010.
// Where the clocks change, a change requested before the night waits for the
// window that exists: 03:00 summer time on the night the clock moves forward.
func TestRetentionAcrossDST_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w, err := analytics.ParseWindow("02:00", "04:00", "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	f := analyticstest.New(t, analyticstest.Options{Window: &w, Start: time.Date(2027, 3, 27, 12, 0, 0, 0, time.UTC)})
	ch, _, err := f.Analytics.SetRetention(ctx, f.Owner, analytics.ClassRawEvents, 120, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2027, 3, 28, 1, 0, 0, 0, time.UTC); !ch.ApplyAt.Equal(want) {
		t.Fatalf("apply at %s, want %s (03:00 CEST)", ch.ApplyAt, want)
	}
	f.Clock.Set(time.Date(2027, 3, 28, 0, 30, 0, 0, time.UTC))
	if err := f.Analytics.Maintain(ctx); err != nil || !settingOf(t, f, analytics.ClassRawEvents).Default {
		t.Fatalf("at 01:30 CET, before the hour that does not exist: %v", err)
	}
	f.Clock.Set(time.Date(2027, 3, 28, 1, 10, 0, 0, time.UTC))
	if err := f.Analytics.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	if s := settingOf(t, f, analytics.ClassRawEvents); s.Default || *s.Days != 120 {
		t.Errorf("at 03:10 CEST: %+v", s)
	}
}
