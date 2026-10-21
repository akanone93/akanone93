// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package analytics is the public boundary of analytics storage (ANL-010,
// ADR-0080, ADR-0084).
//
// The core stores runtime events and their rollups in PostgreSQL. An
// installation that outgrows it, at roughly ten million events a day or a
// million active devices, runs the enterprise edition's ClickHouse adapter
// instead. The adapter implements Store and is handed to the server through
// Opener, so everything that ingests, queries, exports or purges analytics
// stays the same.
//
// The package is outside internal/ and uses only the standard library: the
// enterprise module is a separate Go module and may import only public
// packages of the core.
//
// A Store keeps three promises. Writing an event again, because the queue
// delivers at least once, stores it once. Percentiles come from the fixed
// bucket Histogram, so every store agrees on them. Rollups of one
// organisation are never visible to another: org is always the first scope
// of a call.
package analytics

import (
	"context"
	"log/slog"
	"time"
)

// Granularity is the width of a rollup bucket.
type Granularity string

// The rollup granularities.
const (
	FiveMinutes Granularity = "5m"
	Hour        Granularity = "1h"
	Day         Granularity = "1d"
)

// Width is the length of a bucket. Day buckets are UTC days.
func (g Granularity) Width() time.Duration {
	switch g {
	case FiveMinutes:
		return 5 * time.Minute
	case Hour:
		return time.Hour
	case Day:
		return 24 * time.Hour
	}
	return 0
}

// Valid reports whether g is a granularity.
func (g Granularity) Valid() bool { return g.Width() > 0 }

// Event is one validated runtime event to store. The ID is assigned when
// the event is accepted, so a write that is repeated stores it once.
type Event struct {
	ID              string
	Name            string
	Time            time.Time
	ReleaseSequence int64
	PluginKey       string
	Route           string
	// Category is the consent category: necessary, analytics or
	// experiments (SEC-161).
	Category string
	// Fields is a flat JSON object of scalars.
	Fields []byte
}

// Batch is the events of one device, with the device's dimensions.
type Batch struct {
	OrganizationID, AppID, EnvironmentID, DeviceID string
	// DeviceClass and Locale are the device's, as its newest
	// session_start reported them; locale is empty without analytics
	// consent.
	DeviceClass, Locale string
	Events              []Event
}

// Dimension is one way to group rollups.
type Dimension string

// The dimensions of a rollup.
const (
	ByEnvironment Dimension = "environment"
	ByRelease     Dimension = "release"
	ByPlugin      Dimension = "plugin"
	ByPage        Dimension = "page"
	ByDeviceClass Dimension = "device_class"
	ByLocale      Dimension = "locale"
)

// Dimensions lists them in order.
func Dimensions() []Dimension {
	return []Dimension{ByEnvironment, ByRelease, ByPlugin, ByPage, ByDeviceClass, ByLocale}
}

// Query asks for the rollups of one event name of one app.
type Query struct {
	AppID string
	// EnvironmentID restricts the query to one environment; empty for all.
	EnvironmentID string
	// Metric is the event name.
	Metric      string
	Granularity Granularity
	// From is inclusive and To exclusive; both are aligned down to the
	// granularity.
	From, To time.Time
	// GroupBy keeps the named dimensions apart; every other dimension is
	// summed. Empty gives one series.
	GroupBy []Dimension
	// Limit bounds the rows read; the result says when it was reached.
	Limit int
}

// Group is the dimension values of a point. A dimension not grouped by is
// empty.
type Group struct {
	EnvironmentID string
	Release       int64
	Plugin        string
	Page          string
	DeviceClass   string
	Locale        string
}

// Point is one bucket of one series.
type Point struct {
	AppID       string
	EventName   string
	Granularity Granularity
	Bucket      time.Time
	Group       Group
	// Events counts the events. ValueCount counts those that carry a
	// duration, and Sum, Min, Max, P50, P90 and P99 describe the
	// durations in milliseconds.
	Events, ValueCount int64
	Sum, Min, Max      float64
	P50, P90, P99      float64
	// Histogram is the bucket counts behind the percentiles, for export.
	Histogram Histogram
}

// Series is the result of a query.
type Series struct {
	Points []Point
	// Truncated says the limit was reached; the points are complete up
	// to, not including, NextFrom.
	Truncated bool
	NextFrom  time.Time
}

// Purge says what a retention run deletes. A zero time deletes nothing.
type Purge struct {
	RawBefore, RollupsBefore time.Time
}

// Purged says what was deleted.
type Purged struct {
	Raw, Rollups int64
}

// Store is the analytics storage (ANL-010).
type Store interface {
	// Prepare readies storage for events up to monthsAhead months after
	// now, such as creating the partitions of the coming months.
	Prepare(ctx context.Context, now time.Time, monthsAhead int) error
	// WriteEvents stores a batch. Writing an event again, because the
	// queue delivers at least once, stores it once.
	WriteEvents(ctx context.Context, b Batch) error
	// Rollup folds the events received before until, and not yet folded,
	// into the rollups of an organisation. Late events correct the
	// buckets they belong to.
	Rollup(ctx context.Context, org string, until time.Time) error
	// Query reads one app's rollups.
	Query(ctx context.Context, org string, q Query) (Series, error)
	// Window returns every rollup of an organisation in [from, to), ungrouped,
	// for export.
	Window(ctx context.Context, org string, g Granularity, from, to time.Time) ([]Point, error)
	// Purge deletes raw events and rollups older than the cut-offs.
	Purge(ctx context.Context, org string, p Purge) (Purged, error)
	// Compact releases storage no organisation needs any more: the
	// partitions that end before the time, which is the oldest cut-off of
	// any organisation.
	Compact(ctx context.Context, before time.Time) (int, error)
}

// Settings is what the server tells an Opener about the installation.
type Settings struct {
	// Log receives the store's warnings; it is never nil.
	Log *slog.Logger
}

// Opener opens the analytics store of an installation. The enterprise
// binary sets one in the server's options; without one the core uses its
// PostgreSQL store. A store that holds resources, such as connections,
// implements io.Closer: the server closes it when the process ends.
type Opener interface {
	// Open returns the store. It is called once, when the process starts,
	// with a context that carries the installation's licence (package
	// edition). It returns a nil Store and no error to leave the core on
	// PostgreSQL, such as when the licence does not allow the adapter.
	Open(ctx context.Context, s Settings) (Store, error)
}
