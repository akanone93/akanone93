// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import ext "github.com/nightCode42/plux3/backend/ext/analytics"

// The storage types are public, so the enterprise edition can implement
// Store; the core uses them under their own names.
type (
	// Granularity is the width of a rollup bucket.
	Granularity = ext.Granularity
	// Event is one validated runtime event to store.
	Event = ext.Event
	// Batch is the events of one device, with the device's dimensions.
	Batch = ext.Batch
	// Dimension is one way to group rollups.
	Dimension = ext.Dimension
	// Query asks for the rollups of one event name of one app.
	Query = ext.Query
	// Group is the dimension values of a point.
	Group = ext.Group
	// Point is one bucket of one series.
	Point = ext.Point
	// Series is the result of a query.
	Series = ext.Series
	// Purge says what a retention run deletes.
	Purge = ext.Purge
	// Purged says what was deleted.
	Purged = ext.Purged
	// Store is the analytics storage (ANL-010).
	Store = ext.Store
	// Histogram counts values by bucket.
	Histogram = ext.Histogram
)

// The rollup granularities.
const (
	FiveMinutes = ext.FiveMinutes
	Hour        = ext.Hour
	Day         = ext.Day
)

// The dimensions of a rollup.
const (
	ByEnvironment = ext.ByEnvironment
	ByRelease     = ext.ByRelease
	ByPlugin      = ext.ByPlugin
	ByPage        = ext.ByPage
	ByDeviceClass = ext.ByDeviceClass
	ByLocale      = ext.ByLocale
)

// Buckets is the number of histogram buckets.
const Buckets = ext.Buckets

// Bounds are the upper edges of the histogram buckets, in milliseconds.
var Bounds = ext.Bounds

// Dimensions lists the dimensions in order.
func Dimensions() []Dimension { return ext.Dimensions() }
