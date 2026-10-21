// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package loadgen

import (
	"math"
	"testing"
	"time"
)

// Verifies: DEP-051.
// The histogram's percentiles are within its bucket ratio of the exact
// ones, and an empty histogram reports zero.
func TestHistogramPercentiles_DEP_051(t *testing.T) {
	t.Parallel()
	var h histogram
	if h.quantile(0.95) != 0 {
		t.Error("an empty histogram must report 0")
	}
	for i := 1; i <= 1000; i++ {
		h.add(time.Duration(i) * time.Millisecond)
	}
	for q, exact := range map[float64]float64{0.5: 500, 0.95: 950, 0.99: 990} {
		got := h.quantile(q)
		if math.Abs(got-exact)/exact > histogramRatio-1+0.001 {
			t.Errorf("p%v = %.1f ms, exact %.1f ms", q*100, got, exact)
		}
	}
	if toMS(h.max) != 1000 || h.quantile(1) > 1000 {
		t.Errorf("max = %v, p100 = %v", toMS(h.max), h.quantile(1))
	}
}

// Verifies: DEP-051.
// The same seed and device give the same jitter; it stays within its
// fraction; devices have their own streams.
func TestJitterIsRepeatableAndBounded_DEP_051(t *testing.T) {
	t.Parallel()
	a, b, other := newPRNG(9, 3), newPRNG(9, 3), newPRNG(9, 4)
	same, differs := true, false
	for range 1000 {
		x, y := a.jittered(time.Minute, 0.2), b.jittered(time.Minute, 0.2)
		same = same && x == y
		differs = differs || x != other.jittered(time.Minute, 0.2)
		if x < 48*time.Second || x > 72*time.Second {
			t.Fatalf("jittered = %v, outside 20%% of a minute", x)
		}
	}
	if !same || !differs {
		t.Errorf("repeatable = %v, differs between devices = %v", same, differs)
	}
	if w := a.within(time.Second); w < 0 || w >= time.Second {
		t.Errorf("within = %v", w)
	}
}

// Verifies: DEP-051.
// Each device gets its own address in 10.0.0.0/8.
func TestAddressOfIsUnique_DEP_051(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for i := range 70000 {
		a := addressOf(i)
		if seen[a] {
			t.Fatalf("device %d repeats %s", i, a)
		}
		seen[a] = true
	}
	if addressOf(0) != "10.0.0.1" {
		t.Errorf("addressOf(0) = %s", addressOf(0))
	}
}
