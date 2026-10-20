// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics_test

import (
	"context"
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/analytics/analyticstest"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// sample is one event the tests write and aggregate themselves.
type sample struct {
	at      time.Time
	release int64
	page    string
	value   float64
}

// write stores samples as screen views of the fixture's device.
func write(t *testing.T, f *analyticstest.Fixture, samples []sample) []string {
	t.Helper()
	b := analytics.Batch{
		OrganizationID: f.Org, AppID: f.App, EnvironmentID: f.Env, DeviceID: f.Device, DeviceClass: "phone", Locale: "de-DE",
	}
	for _, s := range samples {
		id, err := f.IDs.New()
		if err != nil {
			t.Fatal(err)
		}
		b.Events = append(b.Events, analytics.Event{
			ID: id, Name: "screen_view", Time: s.at, ReleaseSequence: s.release, Route: s.page, Category: "analytics",
			Fields: fmt.Appendf(nil, `{"duration_ms": %g}`, s.value),
		})
	}
	if err := f.Store.WriteEvents(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(b.Events))
	for i, e := range b.Events {
		ids[i] = e.ID
	}
	return ids
}

// rollUp folds everything written so far, whatever the wall clock says.
func rollUp(t *testing.T, f *analyticstest.Fixture, until time.Time) {
	t.Helper()
	if err := f.Store.Rollup(context.Background(), f.Org, until); err != nil {
		t.Fatal(err)
	}
}

// expectation is what the raw samples add up to in one bucket.
type expectation struct {
	events   int64
	sum      float64
	min, max float64
	hist     analytics.Histogram
}

func aggregate(samples []sample, width time.Duration, key func(sample) string) map[string]*expectation {
	out := map[string]*expectation{}
	for _, s := range samples {
		k := s.at.Truncate(width).Format(time.RFC3339) + "|" + key(s)
		e := out[k]
		if e == nil {
			e = &expectation{min: math.Inf(1), max: math.Inf(-1)}
			out[k] = e
		}
		e.events++
		e.sum += s.value
		e.min, e.max = math.Min(e.min, s.value), math.Max(e.max, s.value)
		e.hist[analytics.BucketOf(s.value)]++
	}
	return out
}

// Verifies: ANL-010.
// Rollups at five minutes, an hour and a day equal what the raw events add
// up to, kept apart by the dimensions asked for, with percentiles estimated
// from the same histogram, and a late event corrects its bucket.
func TestRollupsEqualRawAggregates_ANL_010(t *testing.T) {
	t.Parallel()
	f := analyticstest.New(t, analyticstest.Options{})
	base := time.Date(2026, 10, 18, 22, 3, 0, 0, time.UTC)
	var samples []sample
	for i := range 240 {
		samples = append(samples, sample{
			at: base.Add(time.Duration(i) * 17 * time.Minute), release: int64(1 + i%2), page: []string{"/home", "/loans", "/cards"}[i%3],
			value: float64((i*37)%1900) + 0.5,
		})
	}
	write(t, f, samples)
	received := time.Now().Add(time.Hour)
	rollUp(t, f, received)

	from, to := base.Truncate(24*time.Hour), base.Add(80*time.Hour)
	for _, g := range []analytics.Granularity{analytics.FiveMinutes, analytics.Hour, analytics.Day} {
		for name, c := range map[string]struct {
			group []analytics.Dimension
			key   func(sample) string
		}{
			"ungrouped":  {nil, func(sample) string { return "" }},
			"by release": {[]analytics.Dimension{analytics.ByRelease}, func(s sample) string { return fmt.Sprint(s.release) }},
			"by page":    {[]analytics.Dimension{analytics.ByPage, analytics.ByRelease}, func(s sample) string { return fmt.Sprint(s.page, s.release) }},
		} {
			series := queryAll(t, f, analytics.Query{AppID: f.App, Metric: "screen_view", Granularity: g, From: from, To: to, GroupBy: c.group})
			want := aggregate(samples, g.Width(), c.key)
			if len(series.Points) != len(want) {
				t.Errorf("%s %s: %d points, want %d", g, name, len(series.Points), len(want))
			}
			for _, p := range series.Points {
				key := p.Bucket.Format(time.RFC3339) + "|" + groupKey(p, c.group)
				w := want[key]
				if w == nil {
					t.Errorf("%s %s: unexpected point %s", g, name, key)
					continue
				}
				if p.Events != w.events || p.ValueCount != w.events || math.Abs(p.Sum-w.sum) > 1e-6 || p.Min != w.min || p.Max != w.max || p.Histogram != w.hist {
					t.Errorf("%s %s %s: got %d events sum %v [%v, %v], want %d sum %v [%v, %v]", g, name, key, p.Events, p.Sum, p.Min, p.Max, w.events, w.sum, w.min, w.max)
				}
				if wantP := w.hist.Quantile(0.9, w.min, w.max); p.P90 != wantP {
					t.Errorf("%s %s %s: p90 = %v, want %v", g, name, key, p.P90, wantP)
				}
			}
		}
	}

	// A late event, received after the first pass, corrects its bucket and
	// the hour and day above it.
	late := sample{at: samples[10].at.Add(time.Minute), release: samples[10].release, page: samples[10].page, value: 7000}
	ids := write(t, f, []sample{late})
	setReceived(t, f, ids[0], received.Add(10*time.Minute))
	rollUp(t, f, received.Add(30*time.Minute))
	samples = append(samples, late)
	series := queryAll(t, f, analytics.Query{AppID: f.App, Metric: "screen_view", Granularity: analytics.Day, From: from, To: to})
	var total int64
	for _, p := range series.Points {
		total += p.Events
	}
	if total != int64(len(samples)) {
		t.Errorf("the days add up to %d events, want %d", total, len(samples))
	}
	if series.Points[0].Max < 7000 && series.Points[1].Max < 7000 {
		t.Errorf("the late event's duration is in no day: %v", series.Points)
	}
}

// queryAll follows the pages of a query to its end.
func queryAll(t *testing.T, f *analyticstest.Fixture, q analytics.Query) analytics.Series {
	t.Helper()
	var all analytics.Series
	for range 1000 {
		s, err := f.Analytics.Query(context.Background(), f.Owner, q)
		if err != nil {
			t.Fatal(err)
		}
		all.Points = append(all.Points, s.Points...)
		if !s.Truncated {
			return all
		}
		q.From = s.NextFrom
	}
	t.Fatal("a query never ended")
	return all
}

func groupKey(p analytics.Point, group []analytics.Dimension) string {
	var out string
	if slices.Contains(group, analytics.ByPage) {
		out += p.Group.Page
	}
	if slices.Contains(group, analytics.ByRelease) {
		out += fmt.Sprint(p.Group.Release)
	}
	return out
}

func setReceived(t *testing.T, f *analyticstest.Fixture, id string, at time.Time) {
	t.Helper()
	err := f.DB.InTx(context.Background(), storage.Tenant{OrganizationID: f.Org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE analytics_events SET received_at = $2 WHERE id = $1", id, at)
		return err //nolint:wrapcheck // a test helper
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Verifies: ANL-010, LIM-001.
// A query reads a page at a time: it never splits a bucket across pages,
// continues from where it stopped, and refuses a bucket larger than a page.
func TestQueryPages_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{})
	base := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	var samples []sample
	for i := range 6 {
		for _, page := range []string{"/a", "/b"} {
			samples = append(samples, sample{at: base.Add(time.Duration(i) * time.Hour), release: 1, page: page, value: 5})
		}
	}
	write(t, f, samples)
	rollUp(t, f, time.Now().Add(time.Hour))
	q := analytics.Query{AppID: f.App, Metric: "screen_view", Granularity: analytics.Hour, From: base, To: base.Add(24 * time.Hour), GroupBy: []analytics.Dimension{analytics.ByPage}, Limit: 5}
	var seen int
	for pages := 0; pages < 10; pages++ {
		s, err := f.Analytics.Query(ctx, f.Owner, q)
		if err != nil {
			t.Fatal(err)
		}
		seen += len(s.Points)
		if len(s.Points)%2 != 0 {
			t.Errorf("a page of %d points split a bucket", len(s.Points))
		}
		if !s.Truncated {
			break
		}
		q.From = s.NextFrom
	}
	if seen != 12 {
		t.Errorf("%d points over all pages, want 12", seen)
	}
	q.From, q.Limit = base, 1
	if _, err := f.Analytics.Query(ctx, f.Owner, q); err == nil {
		t.Error("a bucket larger than the page was returned")
	}
}

// Verifies: ANL-010.
// Partitions are created ahead by the worker, idempotently and across a year
// boundary, and an event lands in the partition of its UTC month.
func TestPartitionsAhead_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{})
	if err := f.Store.Prepare(ctx, time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC), 3); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"analytics_events_202610", "analytics_events_202611", "analytics_events_202612", "analytics_events_202701", "analytics_events_202702"} {
		if !slices.Contains(f.Partitions(t), name) {
			t.Errorf("partition %s was not created: %v", name, f.Partitions(t))
		}
	}
	before := len(f.Partitions(t))
	if err := f.Store.Prepare(ctx, time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC), 3); err != nil {
		t.Fatalf("a second Prepare: %v", err)
	}
	if err := f.Store.Prepare(ctx, time.Date(2026, 12, 20, 0, 0, 0, 0, time.UTC), 3); err != nil {
		t.Fatal(err)
	}
	if after := f.Partitions(t); len(after) < before || !slices.Contains(after, "analytics_events_202703") {
		t.Errorf("rolling into December did not add March: %v", after)
	}

	last := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	first := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	write(t, f, []sample{{at: last, value: 1}, {at: first, value: 1}, {at: time.Date(2031, 6, 1, 0, 0, 0, 0, time.UTC), value: 1}})
	got := map[string]string{}
	err := f.DB.InTx(ctx, storage.Tenant{OrganizationID: f.Org}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT time::text, tableoid::regclass::text FROM analytics_events ORDER BY time")
		if err != nil {
			return err //nolint:wrapcheck // a test
		}
		defer rows.Close()
		for rows.Next() {
			var at, part string
			if err := rows.Scan(&at, &part); err != nil {
				return err //nolint:wrapcheck // a test
			}
			got[at[:10]] = part
		}
		return rows.Err() //nolint:wrapcheck // a test
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"2026-12-31": "analytics_events_202612", "2027-01-01": "analytics_events_202701", "2031-06-01": "analytics_events_203106"}
	for day, part := range want {
		if got[day] != part {
			t.Errorf("the event of %s is in %q, want %q", day, got[day], part)
		}
	}
}

// Verifies: ANL-010.
// The histogram bucket the database folds into rollups is the bucket the
// Go code estimates percentiles with.
func TestHistogramBoundsMatchTheDatabase_ANL_010(t *testing.T) {
	t.Parallel()
	f := analyticstest.New(t, analyticstest.Options{})
	err := f.DB.InTx(context.Background(), storage.Tenant{}, func(ctx context.Context, tx pgx.Tx) error {
		values := []float64{-1, 0, 0.99, 1, 1.5, 2, 4.9, 5, 99, 100, 999.9, 1000, 59999, 60000, 1e9}
		for _, v := range values {
			var db int
			if err := tx.QueryRow(ctx, "SELECT plux_value_bucket($1::double precision)", v).Scan(&db); err != nil {
				return err //nolint:wrapcheck // a test
			}
			if db != analytics.BucketOf(v) {
				t.Errorf("plux_value_bucket(%v) = %d, BucketOf = %d", v, db, analytics.BucketOf(v))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
