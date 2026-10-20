// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Aggregation is how the rollups of a window are reduced to one number.
type Aggregation string

// The aggregations. Count adds up events; the others describe the
// durations that events carry (render_perf, action_run, api_call and so
// on), and a window without durations has no value for them.
const (
	AggCount Aggregation = "count"
	AggSum   Aggregation = "sum"
	AggAvg   Aggregation = "avg"
	AggMin   Aggregation = "min"
	AggMax   Aggregation = "max"
	AggP50   Aggregation = "p50"
	AggP90   Aggregation = "p90"
	AggP99   Aggregation = "p99"
)

// Aggregations lists them in order.
func Aggregations() []Aggregation {
	return []Aggregation{AggCount, AggSum, AggAvg, AggMin, AggMax, AggP50, AggP90, AggP99}
}

// Comparison relates the aggregated value to the threshold.
type Comparison string

// The comparisons.
const (
	Greater      Comparison = ">"
	GreaterEqual Comparison = ">="
	Less         Comparison = "<"
	LessEqual    Comparison = "<="
)

// Comparisons lists them in order.
func Comparisons() []Comparison { return []Comparison{Greater, GreaterEqual, Less, LessEqual} }

// holds reports whether value stands in the relation to threshold.
func (c Comparison) holds(value, threshold float64) bool {
	switch c {
	case Greater:
		return value > threshold
	case GreaterEqual:
		return value >= threshold
	case Less:
		return value < threshold
	case LessEqual:
		return value <= threshold
	}
	return false
}

// Severity says how much attention a firing rule needs.
type Severity string

// The severities, the same words as the notification severities.
const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Severities lists them in order.
func Severities() []Severity { return []Severity{SeverityInfo, SeverityWarning, SeverityCritical} }

// Bounds of a rule that follow from the rollups it reads: a window is a
// whole number of rollup buckets, so the bucket width grows with the
// window, and a window cannot be longer than a month of daily buckets.
const (
	fiveMinuteWindowMax = 6 * time.Hour
	hourWindowMax       = 7 * 24 * time.Hour
	maxWindow           = 30 * 24 * time.Hour
	maxFor              = 24 * time.Hour
	maxNameBytes        = 100
	maxMetricBytes      = 64
	maxFilterValueBytes = 128
)

// Rule is an alert rule (ANL-030): the aggregation of one event name's
// rollups over a window, compared with a threshold. It fires once the
// comparison has held for For, and resolves when it stops holding.
type Rule struct {
	ID    string
	AppID string
	Name  string
	// Metric is the event name.
	Metric string
	// EnvironmentID narrows the rule to one environment; empty for all.
	// Environment is that environment's key, which is what a caller names it
	// by and what a stored rule is shown with.
	EnvironmentID string
	Environment   string
	// Filters maps a dimension (not the environment) to the value a series
	// must have; the release is its sequence number in decimal.
	Filters     map[Dimension]string
	Aggregation Aggregation
	Window      time.Duration
	Comparison  Comparison
	Threshold   float64
	For         time.Duration
	Severity    Severity
	Enabled     bool
	Revision    int64
	UpdatedBy   string
	UpdatedAt   time.Time
}

// granularity is the rollup width the rule reads: the finest one that keeps
// a window to at most a few hundred buckets.
func (r Rule) granularity() Granularity {
	switch {
	case r.Window <= fiveMinuteWindowMax:
		return FiveMinutes
	case r.Window <= hourWindowMax:
		return Hour
	}
	return Day
}

// Validate checks a rule's definition.
func (r Rule) Validate() error {
	if r.Name == "" || len(r.Name) > maxNameBytes {
		return plxerr.New(plxerr.InvalidFormat, "the rule name is 1 to %d bytes", maxNameBytes)
	}
	if r.Metric == "" || len(r.Metric) > maxMetricBytes {
		return plxerr.New(plxerr.InvalidFormat, "the metric is an event name of 1 to %d bytes", maxMetricBytes)
	}
	if !slices.Contains(Aggregations(), r.Aggregation) {
		return plxerr.New(plxerr.InvalidEnumValue, "%q is not an aggregation; use %v", r.Aggregation, Aggregations())
	}
	if !slices.Contains(Comparisons(), r.Comparison) {
		return plxerr.New(plxerr.InvalidEnumValue, "%q is not a comparison; use %v", r.Comparison, Comparisons())
	}
	if !slices.Contains(Severities(), r.Severity) {
		return plxerr.New(plxerr.InvalidEnumValue, "%q is not a severity; use %v", r.Severity, Severities())
	}
	if math.IsNaN(r.Threshold) || math.IsInf(r.Threshold, 0) {
		return plxerr.New(plxerr.OutOfRange, "the threshold is not a finite number")
	}
	if err := r.validateTimes(); err != nil {
		return err
	}
	return r.validateFilters()
}

// validateTimes checks the window and the for-duration.
func (r Rule) validateTimes() error {
	width := r.granularity().Width()
	if r.Window < FiveMinutes.Width() || r.Window > maxWindow || r.Window%width != 0 {
		return plxerr.New(plxerr.OutOfRange,
			"the window is between 5 minutes and %d days, in whole %s buckets (5 minutes up to 6 hours, hours up to 7 days, days beyond)",
			int(maxWindow/(24*time.Hour)), width)
	}
	if r.For < 0 || r.For > maxFor || r.For%time.Minute != 0 {
		return plxerr.New(plxerr.OutOfRange, "the for-duration is a whole number of minutes up to %d hours", int(maxFor/time.Hour))
	}
	return nil
}

// validateFilters checks the filter dimensions and values.
func (r Rule) validateFilters() error {
	for d, v := range r.Filters {
		if d == ByEnvironment || !slices.Contains(Dimensions(), d) {
			return plxerr.New(plxerr.InvalidEnumValue, "%q is not a filter dimension; use release, plugin, page, device_class or locale", d)
		}
		if v == "" || len(v) > maxFilterValueBytes {
			return plxerr.New(plxerr.InvalidFormat, "the %s filter is a value of 1 to %d bytes", d, maxFilterValueBytes)
		}
		if d == ByRelease {
			if n, err := strconv.ParseInt(v, 10, 64); err != nil || n < 1 {
				return plxerr.New(plxerr.InvalidFormat, "the release filter is a release sequence number")
			}
		}
	}
	return nil
}
