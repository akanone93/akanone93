// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"context"
	"strings"
	"testing"
	"time"

	ext "github.com/nightCode42/plux3/backend/ext/analytics"
	"github.com/nightCode42/plux3/backend/ext/analytics/storetest"
)

const (
	testOrg    = "0197f000-0000-7000-8000-000000000001"
	testApp    = "0197f000-0000-7000-8000-000000000002"
	testEnv    = "0197f000-0000-7000-8000-000000000003"
	testDevice = "0197f000-0000-7000-8000-000000000004"
	// hostile is an identifier an attacker might send.
	hostile = "x'; DROP TABLE analytics_events; --"
)

// subjectFor returns the conformance subject of a store.
func subjectFor(s *Store, until time.Time) storetest.Subject {
	return storetest.Subject{
		Store: s, Organization: testOrg, App: testApp, Environment: testEnv, Device: testDevice,
		Rollup: func(ctx context.Context) error { return s.Rollup(ctx, testOrg, until) },
	}
}

// Verifies: ANL-010.
// Over an in-memory stand-in for ClickHouse, the store answers the
// conformance queries with the series the PostgreSQL store returns for the
// same events: the folding of five-minute rollups into hours and days, the
// grouping, the percentiles and the dropping of a repeated event agree.
func TestStoreConformance_ANL_010(t *testing.T) {
	t.Parallel()
	s, _, now := testStore(t)
	storetest.Conformance(t, subjectFor(s, now.Add(time.Hour)))
}

// Verifies: ANL-010.
// A second roll-up with nothing new reads nothing more, and an event that
// arrives late corrects its bucket and the hour and day above it.
func TestRollup_LateEventCorrectsBuckets_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, fake, now := testStore(t)
	at := time.Date(2026, 10, 20, 10, 1, 0, 0, time.UTC)
	batch := func(id string, ms string) ext.Batch {
		return ext.Batch{
			OrganizationID: testOrg, AppID: testApp, EnvironmentID: testEnv, DeviceID: testDevice,
			Events: []ext.Event{{ID: id, Name: "screen_view", Time: at, Fields: []byte(ms)}},
		}
	}
	if err := s.WriteEvents(ctx, batch("0197f000-0000-7000-8000-0000000000a1", `{"duration_ms": 10}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Rollup(ctx, testOrg, now); err != nil {
		t.Fatal(err)
	}
	before := len(fake.calls)
	if err := s.Rollup(ctx, testOrg, now); err != nil {
		t.Fatal(err)
	}
	if got := len(fake.calls) - before; got != 1 {
		t.Errorf("a roll-up with nothing new sent %d statements, want only the watermark read", got)
	}
	later := now.Add(time.Hour)
	s.now = func() time.Time { return later }
	if err := s.WriteEvents(ctx, batch("0197f000-0000-7000-8000-0000000000a2", `{"duration_ms": 30}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Rollup(ctx, testOrg, later.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, g := range []ext.Granularity{ext.FiveMinutes, ext.Hour, ext.Day} {
		pts, err := s.Window(ctx, testOrg, g, at.Add(-24*time.Hour), at.Add(24*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if len(pts) != 1 || pts[0].Events != 2 || pts[0].Sum != 40 || pts[0].Max != 30 {
			t.Errorf("%s rollups = %+v, want one point of two events summing 40", g, pts)
		}
	}
}

// Verifies: ANL-010.
// Versions rise even when the clock stands still, so a recomputed bucket
// always replaces the earlier row.
func TestNextVersion_Monotonic_ANL_010(t *testing.T) {
	t.Parallel()
	s, _, _ := testStore(t)
	prev := s.nextVersion()
	for range 5 {
		next := s.nextVersion()
		if next <= prev {
			t.Fatalf("version %d does not follow %d", next, prev)
		}
		prev = next
	}
}

// Verifies: ANL-010, SEC-161.
// A hostile identifier is refused before any statement is sent, in every
// method that takes one.
func TestStore_HostileIdentifiersSendNothing_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	calls := map[string]func(s *Store) error{
		"Rollup": func(s *Store) error { return s.Rollup(ctx, hostile, now) },
		"Query": func(s *Store) error {
			_, err := s.Query(ctx, hostile, ext.Query{AppID: testApp, Metric: "m", Granularity: ext.Hour, From: now, To: now, Limit: 1})
			return err
		},
		"Query app": func(s *Store) error {
			_, err := s.Query(ctx, testOrg, ext.Query{AppID: hostile, Metric: "m", Granularity: ext.Hour, From: now, To: now, Limit: 1})
			return err
		},
		"Query environment": func(s *Store) error {
			_, err := s.Query(ctx, testOrg, ext.Query{AppID: testApp, EnvironmentID: hostile, Metric: "m", Granularity: ext.Hour, From: now, To: now, Limit: 1})
			return err
		},
		"Window": func(s *Store) error { _, err := s.Window(ctx, hostile, ext.Hour, now, now); return err },
		"Purge":  func(s *Store) error { _, err := s.Purge(ctx, hostile, ext.Purge{RawBefore: now}); return err },
		"Events": func(s *Store) error {
			_, _, err := s.Events(ctx, hostile, now, now, Cursor{}, 1)
			return err
		},
		"Events cursor": func(s *Store) error {
			_, _, err := s.Events(ctx, testOrg, now, now, Cursor{Time: now, ID: hostile}, 1)
			return err
		},
		"WriteEvents": func(s *Store) error {
			return s.WriteEvents(ctx, ext.Batch{OrganizationID: hostile, AppID: testApp, EnvironmentID: testEnv, DeviceID: testDevice, Events: []ext.Event{{ID: testOrg}}})
		},
		"WriteEvents event": func(s *Store) error {
			return s.WriteEvents(ctx, ext.Batch{OrganizationID: testOrg, AppID: testApp, EnvironmentID: testEnv, DeviceID: testDevice, Events: []ext.Event{{ID: hostile}}})
		},
	}
	for name, run := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, fake, _ := testStore(t)
			err := run(s)
			if err == nil {
				t.Fatal("the hostile identifier was accepted")
			}
			if strings.Contains(err.Error(), hostile) {
				t.Errorf("the error quotes the identifier: %v", err)
			}
			if len(fake.calls) != 0 {
				t.Errorf("%d statements were sent: %v", len(fake.calls), fake.calls[0].query)
			}
		})
	}
}

// Verifies: ANL-010, SEC-161.
// The organisation is a bound parameter in every query, and appears in no
// query text, whatever the method.
func TestStore_OrganizationIsBound_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, fake, now := testStore(t)
	steps := []func() error{
		func() error { return s.Rollup(ctx, testOrg, now.Add(time.Hour)) },
		func() error {
			_, err := s.Query(ctx, testOrg, ext.Query{AppID: testApp, EnvironmentID: testEnv, Metric: "m", Granularity: ext.Hour, From: now, To: now, Limit: 5})
			return err
		},
		func() error { _, err := s.Window(ctx, testOrg, ext.Day, now, now); return err },
		func() error { _, _, err := s.Events(ctx, testOrg, now, now, Cursor{}, 5); return err },
		func() error {
			_, err := s.Purge(ctx, testOrg, ext.Purge{RawBefore: now, RollupsBefore: now})
			return err
		},
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	checked := 0
	for _, c := range fake.calls {
		if c.args == nil {
			continue
		}
		checked++
		if strings.Contains(c.query, testOrg) {
			t.Errorf("the organisation is written into the query: %s", c.query)
		}
		if !strings.Contains(c.query, "{org:UUID}") || c.namedArg("org") != testOrg {
			t.Errorf("the query does not bind the organisation: %s", c.query)
		}
	}
	if checked < 8 {
		t.Errorf("only %d parameterised statements were checked", checked)
	}
}

// Verifies: ANL-010.
// The raw aggregation query carries the histogram bounds of the public
// package, in order, so the two stores bucket durations alike.
func TestAggregateRawQuery_Bounds_ANL_010(t *testing.T) {
	t.Parallel()
	q := aggregateRawQuery()
	want := "[1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 30000, 60000]"
	if !strings.Contains(q, want) {
		t.Errorf("the query lacks the bounds %s:\n%s", want, q)
	}
	for _, p := range []string{"{org:UUID}", "{from:DateTime64(6)}", "{to:DateTime64(6)}", "LIMIT 1 BY id, time"} {
		if !strings.Contains(q, p) {
			t.Errorf("the query lacks %q", p)
		}
	}
}

// Verifies: ANL-010.
// Purge deletes only the organisation's rows before the cut-offs and counts
// them; a zero cut-off deletes nothing.
func TestPurge_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, fake, now := testStore(t)
	other := "0197f000-0000-7000-8000-0000000000ff"
	for _, org := range []string{testOrg, other} {
		for i, id := range []string{"0197f000-0000-7000-8000-0000000000b1", "0197f000-0000-7000-8000-0000000000b2"} {
			err := s.WriteEvents(ctx, ext.Batch{
				OrganizationID: org, AppID: testApp, EnvironmentID: testEnv, DeviceID: testDevice,
				Events: []ext.Event{{ID: id, Name: "m", Time: now.Add(time.Duration(i) * -48 * time.Hour)}},
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	got, err := s.Purge(ctx, testOrg, ext.Purge{})
	if err != nil || got != (ext.Purged{}) || len(fake.events) != 4 {
		t.Fatalf("a zero purge = %+v, %v with %d events left", got, err, len(fake.events))
	}
	got, err = s.Purge(ctx, testOrg, ext.Purge{RawBefore: now.Add(-24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Raw != 1 || len(fake.events) != 3 {
		t.Errorf("Purge = %+v with %d events left, want one event of the organisation gone", got, len(fake.events))
	}
}

// Verifies: ANL-010.
// Compact drops the monthly partitions that end at or before the time and
// names them from the parsed month, never from the server's text.
func TestCompact_DropsOldPartitions_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, fake, now := testStore(t)
	for i, id := range []string{"0197f000-0000-7000-8000-0000000000c1", "0197f000-0000-7000-8000-0000000000c2", "0197f000-0000-7000-8000-0000000000c3"} {
		err := s.WriteEvents(ctx, ext.Batch{
			OrganizationID: testOrg, AppID: testApp, EnvironmentID: testEnv, DeviceID: testDevice,
			Events: []ext.Event{{ID: id, Name: "m", Time: now.AddDate(0, -i*2, 0)}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	dropped, err := s.Compact(ctx, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 2 {
		t.Errorf("Compact dropped %d partitions, want June and August", dropped)
	}
	last := fake.calls[len(fake.calls)-1].query
	if last != "ALTER TABLE analytics_events DROP PARTITION ID '202608'" {
		t.Errorf("the statement was %q", last)
	}
	for _, id := range []string{"2026-1", "202610'; --", "20261x", ""} {
		if _, ok := partitionEnd(id); ok {
			t.Errorf("partitionEnd accepted %q", id)
		}
	}
}

// Verifies: ANL-010.
// Events are paged in order from a cursor, each once, and the page that is
// shorter than the limit is the last.
func TestEvents_Paging_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _, now := testStore(t)
	var events []ext.Event
	for i := range 5 {
		events = append(events, ext.Event{ID: "0197f000-0000-7000-8000-0000000001" + string(rune('0'+i)) + "0", Name: "m", Time: now.Add(time.Duration(i) * time.Minute), Fields: []byte("{}")})
	}
	b := ext.Batch{OrganizationID: testOrg, AppID: testApp, EnvironmentID: testEnv, DeviceID: testDevice, Events: events}
	if err := s.WriteEvents(ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteEvents(ctx, b); err != nil {
		t.Fatal(err)
	}
	var seen []string
	var cursor Cursor
	for {
		page, next, err := s.Events(ctx, testOrg, now, now.Add(time.Hour), cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range page {
			seen = append(seen, e.ID)
		}
		if len(page) < 2 {
			break
		}
		cursor = next
	}
	if len(seen) != 5 {
		t.Fatalf("paging returned %d events, want 5: %v", len(seen), seen)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i-1] >= seen[i] {
			t.Errorf("events are out of order: %v", seen)
		}
	}
	if _, _, err := s.Events(ctx, testOrg, now, now, Cursor{}, 0); err == nil {
		t.Error("a zero limit was accepted")
	}
}

// Verifies: ANL-010.
// The store closes its connection when the server closes it.
func TestStore_Close_ANL_010(t *testing.T) {
	t.Parallel()
	s, fake, _ := testStore(t)
	if err := s.Close(); err != nil || !fake.closed {
		t.Errorf("Close = %v, closed = %v", err, fake.closed)
	}
}

// Verifies: ANL-010.
// Prepare creates the three tables, partitioned by month for events.
func TestPrepare_CreatesTables_ANL_010(t *testing.T) {
	t.Parallel()
	s, fake, now := testStore(t)
	if err := s.Prepare(context.Background(), now, 3); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 3 {
		t.Fatalf("Prepare sent %d statements, want 3", len(fake.calls))
	}
	if !strings.Contains(fake.calls[0].query, "PARTITION BY toYYYYMM(time)") {
		t.Errorf("the events table is not partitioned by month:\n%s", fake.calls[0].query)
	}
	for _, c := range fake.calls {
		if !strings.HasPrefix(c.query, "CREATE TABLE IF NOT EXISTS ") || strings.Contains(strings.ToUpper(c.query), " TTL ") {
			t.Errorf("unexpected statement: %s", c.query)
		}
	}
}
