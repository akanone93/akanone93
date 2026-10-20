// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"errors"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// steady is a release with 10000 sessions, 100 of which crashed, and a few
// errors.
func steady() Counts {
	return Counts{Sessions: 10000, Crashed: 100, Errors: 500, Actions: 4000, ActionErrors: 40, Calls: 2000, CallErrors: 20}
}

func strict() Gate {
	return Gate{
		MinSample: 1000, MaxCrashFreeDrop: 50, MaxErrorRateIncrease: 200,
		MaxActionErrorRateIncrease: 50, MaxFunctionErrorRateIncrease: 50,
	}
}

// Verifies: REL-053.
func TestJudge_REL_053_NoRegression(t *testing.T) {
	v := Judge(strict(), steady(), steady())
	if !v.Decided || v.Failed() {
		t.Errorf("identical releases = %+v, want decided and healthy", v)
	}
	better := steady()
	better.Crashed, better.Errors = 10, 100
	if v := Judge(strict(), better, steady()); !v.Decided || v.Failed() {
		t.Errorf("a better candidate = %+v, want healthy", v)
	}
}

// Verifies: REL-053.
func TestJudge_REL_053_CrashFreeSessions(t *testing.T) {
	cand := steady()
	cand.Crashed = 160 // 98.40% against 99.00%: a 60 basis point drop
	v := Judge(strict(), cand, steady())
	if !v.Failed() || len(v.Regressions) != 1 || v.Regressions[0].Metric != "crashFreeSessions" {
		t.Fatalf("verdict = %+v, want a crash-free regression", v)
	}
	r := v.Regressions[0]
	if r.Worsening < 59.9 || r.Worsening > 60.1 || r.Threshold != 50 || r.SampleSize != 10000 {
		t.Errorf("regression = %+v", r)
	}
	// Exactly at the threshold is allowed; only worse than it fails.
	cand.Crashed = 150
	if v := Judge(strict(), cand, steady()); v.Failed() {
		t.Errorf("a drop equal to the threshold failed: %+v", v)
	}
}

// Verifies: REL-053.
func TestJudge_REL_053_ErrorRates(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Counts)
		metric string
	}{
		{"plux errors", func(c *Counts) { c.Errors = 3000 }, "errorRate"},
		{"action errors", func(c *Counts) { c.ActionErrors = 400 }, "actionErrorRate"},
		{"function errors", func(c *Counts) { c.CallErrors = 300 }, "functionErrorRate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cand := steady()
			tc.mutate(&cand)
			v := Judge(strict(), cand, steady())
			if !v.Failed() || v.Regressions[0].Metric != tc.metric {
				t.Errorf("verdict = %+v, want a %s regression", v, tc.metric)
			}
		})
	}
}

// Verifies: REL-053.
func TestJudge_REL_053_CustomMetric(t *testing.T) {
	g := Gate{MinSample: 1000, Custom: []CustomGate{{Event: "checkout_failed", MaxIncreaseBasisPoints: 100}}}
	prev, cand := steady(), steady()
	prev.Custom = map[string]int64{"checkout_failed": 50}
	cand.Custom = map[string]int64{"checkout_failed": 300} // 3% against 0.5% of sessions
	v := Judge(g, cand, prev)
	if !v.Failed() || v.Regressions[0].Metric != "custom:checkout_failed" {
		t.Fatalf("verdict = %+v", v)
	}
	cand.Custom["checkout_failed"] = 120
	if Judge(g, cand, prev).Failed() {
		t.Error("an increase within the threshold failed")
	}
}

// A sample that is too small decides nothing, however bad the numbers: a
// handful of early sessions must not roll a release back.
//
// Verifies: REL-053.
func TestJudge_REL_053_TooSmallASampleDecidesNothing(t *testing.T) {
	cand := Counts{Sessions: 10, Crashed: 10, Errors: 100}
	v := Judge(strict(), cand, steady())
	if v.Decided || v.Failed() {
		t.Errorf("a candidate of 10 sessions = %+v, want undecided", v)
	}
	prev := Counts{Sessions: 10}
	if v := Judge(strict(), steady(), prev); v.Decided {
		t.Errorf("a previous release of 10 sessions = %+v, want undecided", v)
	}
	if v := Judge(strict(), Counts{}, Counts{}); v.Decided || v.Failed() {
		t.Errorf("no events at all = %+v, want undecided", v)
	}
}

// Each rate needs its own sample: plenty of sessions but few actions leaves
// the action error rate out, while the session-based checks still decide.
//
// Verifies: REL-053.
func TestJudge_REL_053_SamplePerMetric(t *testing.T) {
	cand := steady()
	cand.Actions, cand.ActionErrors = 20, 20
	prev := steady()
	v := Judge(strict(), cand, prev)
	if !v.Decided || v.Failed() {
		t.Errorf("verdict = %+v, want decided on sessions and healthy", v)
	}
	g := Gate{MinSample: 1000, MaxActionErrorRateIncrease: 50}
	if v := Judge(g, cand, prev); v.Decided {
		t.Errorf("a gate on actions alone with few actions = %+v, want undecided", v)
	}
}

// A threshold of zero switches its check off.
//
// Verifies: REL-053.
func TestJudge_REL_053_ZeroThresholdIsOff(t *testing.T) {
	cand := steady()
	cand.Crashed, cand.Errors = 5000, 9000
	if v := Judge(Gate{MinSample: 1}, cand, steady()); v.Decided || v.Failed() {
		t.Errorf("an unarmed gate = %+v", v)
	}
}

// Verifies: REL-053.
func TestGate_REL_053_ValidateAndArmed(t *testing.T) {
	if (Gate{}).Armed() || !strict().Armed() || !(Gate{Custom: []CustomGate{{Event: "e", MaxIncreaseBasisPoints: 1}}}).Armed() {
		t.Error("Armed is wrong")
	}
	for _, g := range []Gate{strict(), {Action: ActionRollback, Custom: []CustomGate{{Event: "e", MaxIncreaseBasisPoints: 10}}}, {}} {
		if err := g.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v", g, err)
		}
	}
	bad := []Gate{
		{MinSample: -1},
		{WindowSeconds: -5},
		{MaxCrashFreeDrop: -1},
		{MaxErrorRateIncrease: -1},
		{Action: "explode"},
		{Custom: []CustomGate{{Event: "", MaxIncreaseBasisPoints: 5}}},
		{Custom: []CustomGate{{Event: "e", MaxIncreaseBasisPoints: 0}}},
		{Custom: []CustomGate{{Event: strings.Repeat("x", 65), MaxIncreaseBasisPoints: 5}}},
	}
	for _, g := range bad {
		var pe *plxerr.Error
		if err := g.Validate(); !errors.As(err, &pe) {
			t.Errorf("Validate(%+v) = %v, want a refusal", g, err)
		}
	}
}
