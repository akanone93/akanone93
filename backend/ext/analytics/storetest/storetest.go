// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package storetest

import (
	"context"
	_ "embed" // the fixture is part of the suite
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/ext/analytics"
)

//go:embed testdata/store_conformance.json
var fixtureJSON []byte

// queryLimit is the row limit of every fixture query; the fixture is far
// below it.
const queryLimit = 1000

// Subject is a store under test with the identifiers the events are
// written for.
type Subject struct {
	Store analytics.Store
	// Organization, App, Environment and Device exist in the store's
	// surroundings, such as the foreign keys of a database.
	Organization, App, Environment, Device string
	// Rollup folds every event written so far into the rollups.
	Rollup func(ctx context.Context) error
}

// fixture is the content of testdata/store_conformance.json.
type fixture struct {
	Description string         `json:"description"`
	From        time.Time      `json:"from"`
	To          time.Time      `json:"to"`
	Events      []fixtureEvent `json:"events"`
	Queries     []fixtureQuery `json:"queries"`
	Expected    []Series       `json:"expected"`
}

type fixtureEvent struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	At      time.Time       `json:"at"`
	Release int64           `json:"release"`
	Plugin  string          `json:"plugin"`
	Route   string          `json:"route"`
	Class   string          `json:"class"`
	Locale  string          `json:"locale"`
	Fields  json.RawMessage `json:"fields"`
}

type fixtureQuery struct {
	Metric      string   `json:"metric"`
	Granularity string   `json:"granularity"`
	GroupBy     []string `json:"group_by"`
}

// Point is a point of a series with symbolic identifiers.
type Point struct {
	Bucket     time.Time       `json:"bucket"`
	Group      analytics.Group `json:"group"`
	Events     int64           `json:"events"`
	ValueCount int64           `json:"value_count"`
	Sum        float64         `json:"sum"`
	Min        float64         `json:"min"`
	Max        float64         `json:"max"`
	P50        float64         `json:"p50"`
	P90        float64         `json:"p90"`
	P99        float64         `json:"p99"`
	Histogram  []int64         `json:"histogram"`
}

// Series is the answer to one fixture query.
type Series struct {
	Points []Point `json:"points"`
}

// Run writes the fixture's events to the store, rolls them up, runs the
// queries and returns the series, one per query. Events are written in
// batches of one device class and locale, as a device reports them.
func Run(ctx context.Context, s Subject) ([]Series, error) {
	f, err := load()
	if err != nil {
		return nil, err
	}
	if err := write(ctx, s, f.Events); err != nil {
		return nil, err
	}
	if err := s.Rollup(ctx); err != nil {
		return nil, fmt.Errorf("storetest: roll up: %w", err)
	}
	out := make([]Series, 0, len(f.Queries))
	for _, q := range f.Queries {
		series, err := s.Store.Query(ctx, s.Organization, analytics.Query{
			AppID: s.App, Metric: q.Metric, Granularity: analytics.Granularity(q.Granularity),
			From: f.From, To: f.To, GroupBy: dimensions(q.GroupBy), Limit: queryLimit,
		})
		if err != nil {
			return nil, fmt.Errorf("storetest: query %s %s: %w", q.Metric, q.Granularity, err)
		}
		if series.Truncated {
			return nil, fmt.Errorf("storetest: query %s %s: the series was truncated", q.Metric, q.Granularity)
		}
		out = append(out, symbolic(series.Points, s))
	}
	return out, nil
}

// Conformance runs the suite and fails the test for every query whose series
// differs from the expected one.
func Conformance(t *testing.T, s Subject) {
	t.Helper()
	f, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Expected) != len(f.Queries) {
		t.Fatalf("the fixture holds %d expected series for %d queries", len(f.Expected), len(f.Queries))
	}
	got, err := Run(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	for i, q := range f.Queries {
		if !reflect.DeepEqual(got[i], f.Expected[i]) {
			want, _ := json.Marshal(f.Expected[i])
			have, _ := json.Marshal(got[i])
			t.Errorf("query %d (%s, %s, by %v):\n got %s\nwant %s", i, q.Metric, q.Granularity, q.GroupBy, have, want)
		}
	}
}

// load decodes the fixture.
func load() (fixture, error) {
	var f fixture
	if err := json.Unmarshal(fixtureJSON, &f); err != nil {
		return fixture{}, fmt.Errorf("storetest: decode the fixture: %w", err)
	}
	return f, nil
}

// write stores the events, one batch per run of events with the same device
// class and locale.
func write(ctx context.Context, s Subject, events []fixtureEvent) error {
	for start := 0; start < len(events); {
		end := start + 1
		for end < len(events) && events[end].Class == events[start].Class && events[end].Locale == events[start].Locale {
			end++
		}
		b := analytics.Batch{
			OrganizationID: s.Organization, AppID: s.App, EnvironmentID: s.Environment, DeviceID: s.Device,
			DeviceClass: events[start].Class, Locale: events[start].Locale,
		}
		for _, e := range events[start:end] {
			b.Events = append(b.Events, analytics.Event{
				ID: e.ID, Name: e.Name, Time: e.At, ReleaseSequence: e.Release, PluginKey: e.Plugin, Route: e.Route,
				Category: "analytics", Fields: []byte(e.Fields),
			})
		}
		if err := s.Store.WriteEvents(ctx, b); err != nil {
			return fmt.Errorf("storetest: write events: %w", err)
		}
		start = end
	}
	return nil
}

// dimensions converts the fixture's dimension names.
func dimensions(names []string) []analytics.Dimension {
	out := make([]analytics.Dimension, len(names))
	for i, n := range names {
		out[i] = analytics.Dimension(n)
	}
	return out
}

// symbolic converts points to the fixture's form: identifiers replaced by
// their symbols.
func symbolic(points []analytics.Point, s Subject) Series {
	out := Series{Points: make([]Point, len(points))}
	for i, p := range points {
		g := p.Group
		if g.EnvironmentID == s.Environment {
			g.EnvironmentID = "env"
		}
		out.Points[i] = Point{
			Bucket: p.Bucket.UTC(), Group: g, Events: p.Events, ValueCount: p.ValueCount, Sum: p.Sum, Min: p.Min, Max: p.Max,
			P50: p.P50, P90: p.P90, P99: p.P99, Histogram: p.Histogram[:],
		}
	}
	return out
}

// Marshal returns the fixture with the given series as its expectations. A
// maintainer regenerates the file from the PostgreSQL store when the fixture
// changes.
func Marshal(series []Series) ([]byte, error) {
	f, err := load()
	if err != nil {
		return nil, err
	}
	f.Expected = series
	out, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		return nil, fmt.Errorf("storetest: encode the fixture: %w", err)
	}
	return append(out, '\n'), nil
}
