// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"math"
	"slices"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Action is what a failed health gate does.
type Action string

// The actions of a failed gate. Pausing is the default: it stops the harm
// from growing and leaves the decision to a person (REL-053).
const (
	ActionPause    Action = "pause"
	ActionRollback Action = "rollback"
)

// CustomGate gates on how often a custom event happens per session.
type CustomGate struct {
	// Event is the event name the analytics rollups count.
	Event string `json:"event"`
	// MaxIncreaseBasisPoints is the most the candidate's events per session
	// may exceed the previous release's, in 1/10000.
	MaxIncreaseBasisPoints int `json:"maxIncreaseBasisPoints"`
}

// Gate is a rollout's health gate (REL-053): thresholds on how much worse
// than the previous release the candidate may be. Every threshold is an
// absolute difference in basis points of a rate; zero switches its check off.
type Gate struct {
	// MinSample is the fewest sessions (actions, function calls for their own
	// rates) each release needs in the window before the gate decides.
	MinSample int64 `json:"minSample,omitempty"`
	// WindowSeconds is how far back the comparison reaches.
	WindowSeconds int64 `json:"windowSeconds,omitempty"`

	MaxCrashFreeDrop             int          `json:"maxCrashFreeDrop,omitempty"`
	MaxErrorRateIncrease         int          `json:"maxErrorRateIncrease,omitempty"`
	MaxActionErrorRateIncrease   int          `json:"maxActionErrorRateIncrease,omitempty"`
	MaxFunctionErrorRateIncrease int          `json:"maxFunctionErrorRateIncrease,omitempty"`
	Custom                       []CustomGate `json:"custom,omitempty"`
	// Action is what a failed gate does; empty is ActionPause.
	Action Action `json:"action,omitempty"`
}

// Window is the length of the comparison window.
func (g Gate) Window() time.Duration { return time.Duration(g.WindowSeconds) * time.Second }

// Armed reports whether the gate checks anything.
func (g Gate) Armed() bool {
	return g.MaxCrashFreeDrop > 0 || g.MaxErrorRateIncrease > 0 ||
		g.MaxActionErrorRateIncrease > 0 || g.MaxFunctionErrorRateIncrease > 0 || len(g.Custom) > 0
}

// Validate checks a gate's numbers and action.
func (g Gate) Validate() error {
	thresholds := []int{
		g.MaxCrashFreeDrop, g.MaxErrorRateIncrease,
		g.MaxActionErrorRateIncrease, g.MaxFunctionErrorRateIncrease,
	}
	for _, c := range g.Custom {
		if c.Event == "" || len(c.Event) > 64 || c.MaxIncreaseBasisPoints <= 0 {
			return plxerr.New(plxerr.InvalidFormat, "a custom gate names an event of up to 64 bytes and a positive threshold")
		}
		thresholds = append(thresholds, c.MaxIncreaseBasisPoints)
	}
	if g.MinSample < 0 || g.WindowSeconds < 0 || slices.ContainsFunc(thresholds, func(t int) bool { return t < 0 }) {
		return plxerr.New(plxerr.OutOfRange, "the thresholds, the sample and the window of a health gate are not negative")
	}
	if g.Action != "" && g.Action != ActionPause && g.Action != ActionRollback {
		return plxerr.New(plxerr.InvalidEnumValue, "%q is not a health action; use pause or rollback", g.Action)
	}
	return nil
}

// Counts are the events of one release in the comparison window, as the
// analytics rollups count them (Q13).
type Counts struct {
	// Sessions are the session_start events; Crashed the fatal_error events.
	Sessions, Crashed int64
	// Errors are the Plux errors reported.
	Errors int64
	// Actions and ActionErrors are the action runs and those that failed;
	// Calls and CallErrors the function calls and those that failed.
	Actions, ActionErrors int64
	Calls, CallErrors     int64
	// Custom counts the events of the gate's custom metrics by name.
	Custom map[string]int64
}

// Regression is one metric that got worse beyond its threshold.
type Regression struct {
	Metric string `json:"metric"`
	// Candidate and Previous are the metric's values, as rates; Worsening is
	// their difference in the harmful direction, in basis points, and
	// Threshold the most the gate allows.
	Candidate  float64 `json:"candidate"`
	Previous   float64 `json:"previous"`
	Worsening  float64 `json:"worseningBasisPoints"`
	Threshold  int     `json:"thresholdBasisPoints"`
	SampleSize int64   `json:"sample"`
}

// Verdict is the outcome of comparing the candidate with the previous release.
type Verdict struct {
	// Decided is false when no metric had a large enough sample in both
	// releases: the gate then does nothing.
	Decided     bool         `json:"decided"`
	Regressions []Regression `json:"regressions,omitempty"`
	Candidate   Counts       `json:"candidate"`
	Previous    Counts       `json:"previous"`
}

// Failed reports whether a decided gate found a regression.
func (v Verdict) Failed() bool { return v.Decided && len(v.Regressions) > 0 }

// metric is one comparison the gate can make: how many events out of how
// many, for each release, and what is harmful.
type metric struct {
	name      string
	threshold int
	// higherWorse is true when a larger value is the harmful direction; a
	// crash-free rate is the opposite.
	higherWorse bool
	cand, prev  rate
}

// rate is a count out of a base.
type rate struct{ events, base int64 }

func (r rate) value() float64 { return float64(r.events) / float64(r.base) }

// Judge compares the candidate's counts with the previous release's under the
// gate (REL-053). A metric takes part only when both releases have at least
// MinSample of its base; if none does, the verdict is not decided. Two
// calls with the same input give the same verdict.
func Judge(g Gate, cand, prev Counts) Verdict {
	v := Verdict{Candidate: cand, Previous: prev}
	for _, m := range metrics(g, cand, prev) {
		if m.threshold <= 0 || m.cand.base < g.MinSample || m.prev.base < g.MinSample || m.cand.base == 0 || m.prev.base == 0 {
			continue
		}
		v.Decided = true
		c, p := m.cand.value(), m.prev.value()
		worse := math.Round((c-p)*Buckets*1e6) / 1e6 // rounded so that a drop equal to the threshold is not read as one beyond it
		if !m.higherWorse {
			worse = -worse
		}
		if worse > float64(m.threshold) {
			v.Regressions = append(v.Regressions, Regression{
				Metric: m.name, Candidate: c, Previous: p, Worsening: worse, Threshold: m.threshold, SampleSize: m.cand.base,
			})
		}
	}
	return v
}

// survived is the sessions that did not end in a fatal error, out of all
// sessions; a release cannot lose more sessions than it had.
func survived(c Counts) rate { return rate{c.Sessions - min(c.Crashed, c.Sessions), c.Sessions} }

// metrics lists the comparisons a gate makes, in a fixed order.
func metrics(g Gate, cand, prev Counts) []metric {
	out := []metric{
		{
			"crashFreeSessions", g.MaxCrashFreeDrop, false,
			survived(cand), survived(prev),
		},
		{
			"errorRate", g.MaxErrorRateIncrease, true,
			rate{cand.Errors, cand.Sessions},
			rate{prev.Errors, prev.Sessions},
		},
		{
			"actionErrorRate", g.MaxActionErrorRateIncrease, true,
			rate{cand.ActionErrors, cand.Actions},
			rate{prev.ActionErrors, prev.Actions},
		},
		{
			"functionErrorRate", g.MaxFunctionErrorRateIncrease, true,
			rate{cand.CallErrors, cand.Calls},
			rate{prev.CallErrors, prev.Calls},
		},
	}
	for _, c := range g.Custom {
		out = append(out, metric{
			"custom:" + c.Event, c.MaxIncreaseBasisPoints, true,
			rate{cand.Custom[c.Event], cand.Sessions},
			rate{prev.Custom[c.Event], prev.Sessions},
		})
	}
	return out
}
