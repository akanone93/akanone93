// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"math"
	"strconv"
	"time"
)

// AlertPhase is where a rule stands.
type AlertPhase string

// The phases of the alert state machine: ok, then pending while the
// condition holds for less than the for-duration, firing once it has held
// that long, and resolved when a firing rule's condition stops holding. A
// resolved rule that breaches again goes through pending like an ok one.
const (
	PhaseOK       AlertPhase = "ok"
	PhasePending  AlertPhase = "pending"
	PhaseFiring   AlertPhase = "firing"
	PhaseResolved AlertPhase = "resolved"
)

// AlertState is the stored state of a rule.
type AlertState struct {
	RuleID, AppID, RuleName string
	Phase                   AlertPhase
	// Since is when the phase began.
	Since time.Time
	// EvaluatedAt is the evaluation window last applied.
	EvaluatedAt time.Time
	// Value is the aggregation at the last evaluation; nil without data.
	Value               *float64
	FiredAt, ResolvedAt time.Time
}

// Notice says what a step of the state machine asks the caller to announce.
type Notice int

// The notices.
const (
	NoNotice Notice = iota
	NoticeFired
	NoticeResolved
)

// Step applies one evaluation to a state and returns the new state and what
// to announce. breach says the comparison holds at the evaluation window
// ending at, and hold is the rule's for-duration.
//
// An evaluation at or before the state's EvaluatedAt is a repeat of one
// already applied and changes nothing, which is what keeps a rule from
// firing twice when two evaluators race, or when a job is retried. A rule
// that stops breaching while still pending returns to ok without a notice,
// so a metric that flickers around its threshold for less than the
// for-duration never reaches anyone.
func Step(prev AlertState, breach bool, value *float64, at time.Time, hold time.Duration) (AlertState, Notice) {
	if !at.After(prev.EvaluatedAt) {
		return prev, NoNotice
	}
	next := prev
	next.EvaluatedAt, next.Value = at, value
	switch {
	case breach && (prev.Phase == PhaseOK || prev.Phase == PhaseResolved):
		next.Phase, next.Since = PhasePending, at
		return fireIfHeld(next, hold, at)
	case breach && prev.Phase == PhasePending:
		return fireIfHeld(next, hold, at)
	case !breach && prev.Phase == PhasePending:
		next.Phase, next.Since = PhaseOK, at
	case !breach && prev.Phase == PhaseFiring:
		next.Phase, next.Since, next.ResolvedAt = PhaseResolved, at, at
		return next, NoticeResolved
	}
	return next, NoNotice
}

// fireIfHeld moves a pending state to firing when its condition has held
// for the for-duration.
func fireIfHeld(s AlertState, hold time.Duration, at time.Time) (AlertState, Notice) {
	if at.Sub(s.Since) < hold {
		return s, NoNotice
	}
	s.Phase, s.Since, s.FiredAt = PhaseFiring, at, at
	return s, NoticeFired
}

// aggregate reduces the points of a window to the rule's value. ok is false
// when the window holds nothing to describe: a duration aggregation over
// events that carry no duration. Count is always defined, and is zero for an
// empty window.
func aggregate(a Aggregation, points []Point) (float64, bool) {
	var events, count int64
	var sum float64
	lo, hi := math.Inf(1), math.Inf(-1)
	var hist Histogram
	for _, p := range points {
		events += p.Events
		if p.ValueCount == 0 {
			continue
		}
		count += p.ValueCount
		sum += p.Sum
		lo, hi = math.Min(lo, p.Min), math.Max(hi, p.Max)
		hist.Add(p.Histogram[:])
	}
	if a == AggCount {
		return float64(events), true
	}
	if count == 0 {
		return 0, false
	}
	switch a {
	case AggSum:
		return sum, true
	case AggAvg:
		return sum / float64(count), true
	case AggMin:
		return lo, true
	case AggMax:
		return hi, true
	case AggP50:
		return hist.Quantile(0.50, lo, hi), true
	case AggP90:
		return hist.Quantile(0.90, lo, hi), true
	case AggP99:
		return hist.Quantile(0.99, lo, hi), true
	}
	return 0, false
}

// matches reports whether a point belongs to the rule's filtered series.
func (r Rule) matches(p Point) bool {
	for d, want := range r.Filters {
		if groupValue(p.Group, d) != want {
			return false
		}
	}
	return true
}

// groupValue is the value of a dimension in a point's group, as the string a
// filter names it by.
func groupValue(g Group, d Dimension) string {
	switch d {
	case ByRelease:
		return strconv.FormatInt(g.Release, 10)
	case ByPlugin:
		return g.Plugin
	case ByPage:
		return g.Page
	case ByDeviceClass:
		return g.DeviceClass
	case ByLocale:
		return g.Locale
	case ByEnvironment:
		return g.EnvironmentID
	}
	return ""
}
