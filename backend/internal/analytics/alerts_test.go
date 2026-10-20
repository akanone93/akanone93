// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"testing"
	"time"
)

// Verifies: ANL-030.
// Each comparison holds exactly where its symbol says, including at the
// threshold.
func TestComparisons_ANL_030(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cmp           Comparison
		below, at, up bool
	}{
		{Greater, false, false, true},
		{GreaterEqual, false, true, true},
		{Less, true, false, false},
		{LessEqual, true, true, false},
	}
	for _, c := range cases {
		for i, v := range []float64{4, 5, 6} {
			want := []bool{c.below, c.at, c.up}[i]
			if got := c.cmp.holds(v, 5); got != want {
				t.Errorf("%g %s 5 = %v, want %v", v, c.cmp, got, want)
			}
		}
	}
	if Comparison("!=").holds(1, 2) {
		t.Error("an unknown comparison held")
	}
}

// step applies a sequence of evaluations one minute apart and returns the
// phases and notices.
func step(hold time.Duration, breaches ...bool) ([]AlertPhase, []Notice) {
	at := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	var s AlertState
	s.Phase = PhaseOK
	var phases []AlertPhase
	var notices []Notice
	for _, b := range breaches {
		at = at.Add(time.Minute)
		var n Notice
		s, n = Step(s, b, nil, at, hold)
		phases, notices = append(phases, s.Phase), append(notices, n)
	}
	return phases, notices
}

// Verifies: ANL-030.
// The state machine goes pending, firing and resolved; the for-duration
// holds a rule back, and a flicker shorter than it never notifies.
func TestStep_ANL_030(t *testing.T) {
	t.Parallel()
	const (
		ok, pending, firing, resolved = PhaseOK, PhasePending, PhaseFiring, PhaseResolved
		none, fired, done             = NoNotice, NoticeFired, NoticeResolved
	)
	cases := []struct {
		name     string
		hold     time.Duration
		breaches []bool
		phases   []AlertPhase
		notices  []Notice
	}{
		{
			"fires at once without a for-duration", 0,
			[]bool{true, true, false},
			[]AlertPhase{firing, firing, resolved},
			[]Notice{fired, none, done},
		},
		{
			"waits for the for-duration", 2 * time.Minute,
			[]bool{true, true, true, true, false},
			[]AlertPhase{pending, pending, firing, firing, resolved},
			[]Notice{none, none, fired, none, done},
		},
		{
			"a flicker never fires", 3 * time.Minute,
			[]bool{true, true, false, true, false},
			[]AlertPhase{pending, pending, ok, pending, ok},
			[]Notice{none, none, none, none, none},
		},
		{
			"a resolved rule fires again through pending", time.Minute,
			[]bool{true, true, false, true, true},
			[]AlertPhase{pending, firing, resolved, pending, firing},
			[]Notice{none, fired, done, none, fired},
		},
		{
			"a quiet rule stays ok", time.Minute,
			[]bool{false, false},
			[]AlertPhase{ok, ok},
			[]Notice{none, none},
		},
	}
	for _, c := range cases {
		phases, notices := step(c.hold, c.breaches...)
		for i := range c.breaches {
			if phases[i] != c.phases[i] || notices[i] != c.notices[i] {
				t.Errorf("%s: step %d = %s/%d, want %s/%d", c.name, i, phases[i], notices[i], c.phases[i], c.notices[i])
			}
		}
	}
}

// Verifies: ANL-030.
// An evaluation of a window already applied changes nothing, so two
// evaluators of one rule fire it once.
func TestStepRepeat_ANL_030(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 19, 10, 1, 0, 0, time.UTC)
	first, n := Step(AlertState{Phase: PhaseOK}, true, nil, at, 0)
	if n != NoticeFired {
		t.Fatalf("first notice = %d", n)
	}
	for _, w := range []time.Time{at, at.Add(-time.Minute)} {
		again, n := Step(first, true, nil, w, 0)
		if n != NoNotice || again != first {
			t.Errorf("a repeat at %s notified (%d) or changed the state", w, n)
		}
	}
}

// Verifies: ANL-030.
// Fingerprints are stable across the noise of devices and differ by code,
// path and frames.
func TestFingerprint_ANL_040(t *testing.T) {
	t.Parallel()
	a := NormaliseFrames("#0  Foo.bar (package:app/src/x.dart:12:5)\n#1  Baz.qux (file:///data/user/0/app/lib/y.dart:7:1)\n\n#2  0x7f12ab Main.run (package:app/main.dart:3)")
	b := NormaliseFrames("#0   Foo.bar (package:app/src/x.dart:99:1)\n#1  Baz.qux (file:///var/mobile/app/lib/y.dart:70:11)\n#2  Main.run (package:app/main.dart:30)")
	if len(a) != 3 || a[0] != "Foo.bar (package:app/src/x.dart)" || a[1] != "Baz.qux (y.dart)" {
		t.Errorf("frames = %q", a)
	}
	if Fingerprint("PLX-4101", "root/0", a) != Fingerprint("PLX-4101", "root/0", b) {
		t.Error("the same error on two devices has two fingerprints")
	}
	base := Fingerprint("PLX-4101", "root/0", a)
	for name, other := range map[string]string{
		"code":   Fingerprint("PLX-4102", "root/0", a),
		"path":   Fingerprint("PLX-4101", "root/1", a),
		"frames": Fingerprint("PLX-4101", "root/0", a[:1]),
	} {
		if other == base {
			t.Errorf("a different %s has the same fingerprint", name)
		}
	}
}
