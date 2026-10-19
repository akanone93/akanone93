// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package analytics_test

import (
	"math"
	"sort"
	"testing"

	"github.com/nightCode42/plux3/backend/ext/analytics"
)

// Verifies: ANL-010.
// A value lands in the bucket whose lower bound it meets or passes, and the
// edges belong to the upper bucket.
func TestBucketOf_ANL_010(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		v    float64
		want int
	}{
		{-5, 0},
		{0, 0},
		{0.999, 0},
		{1, 1},
		{1.5, 1},
		{2, 2},
		{4.99, 2},
		{5, 3},
		{999, 9},
		{1000, 10},
		{59999, 14},
		{60000, 15},
		{1e12, 15},
	} {
		if got := analytics.BucketOf(c.v); got != c.want {
			t.Errorf("BucketOf(%v) = %d, want %d", c.v, got, c.want)
		}
	}
	if analytics.Buckets != len(analytics.Bounds)+1 {
		t.Errorf("%d buckets for %d bounds", analytics.Buckets, len(analytics.Bounds))
	}
}

// observe fills a histogram from values and returns it with their range.
func observe(values []float64) (h analytics.Histogram, lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, v := range values {
		h[analytics.BucketOf(v)]++
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	return h, lo, hi
}

// Verifies: ANL-010.
// A percentile estimate stays inside the observed range and within one
// bucket of the exact percentile, whatever the distribution.
func TestQuantileErrorIsBoundedByABucket_ANL_010(t *testing.T) {
	t.Parallel()
	distributions := map[string][]float64{
		"uniform": nil, "skewed": nil, "single": {42, 42, 42, 42}, "two": {3, 900},
	}
	for i := 1; i <= 500; i++ {
		distributions["uniform"] = append(distributions["uniform"], float64(i))
		distributions["skewed"] = append(distributions["skewed"], float64(i*i)/50)
	}
	for name, values := range distributions {
		h, lo, hi := observe(values)
		sorted := append([]float64(nil), values...)
		sort.Float64s(sorted)
		for _, q := range []float64{0.5, 0.9, 0.99} {
			got := h.Quantile(q, lo, hi)
			exact := sorted[int(math.Ceil(q*float64(len(sorted))))-1]
			if got < lo || got > hi {
				t.Errorf("%s p%.0f = %v outside [%v, %v]", name, q*100, got, lo, hi)
			}
			b := analytics.BucketOf(exact)
			edgeLo, edgeHi := lo, hi
			if b > 0 {
				edgeLo = math.Max(analytics.Bounds[b-1], lo)
			}
			if b < len(analytics.Bounds) {
				edgeHi = math.Min(analytics.Bounds[b], hi)
			}
			if got < edgeLo-1e-9 || got > edgeHi+1e-9 {
				t.Errorf("%s p%.0f = %v, exact %v is in [%v, %v]", name, q*100, got, exact, edgeLo, edgeHi)
			}
		}
	}
}

// Verifies: ANL-010.
// Percentiles are monotonic in q, an empty histogram gives zero, and q is
// clamped to 0 and 1.
func TestQuantileEdges_ANL_010(t *testing.T) {
	t.Parallel()
	var empty analytics.Histogram
	if got := empty.Quantile(0.5, 0, 0); got != 0 {
		t.Errorf("empty p50 = %v", got)
	}
	h, lo, hi := observe([]float64{3, 7, 15, 30, 80, 150, 400, 700, 1500, 4000})
	prev := -1.0
	for q := 0.0; q <= 1.0; q += 0.05 {
		got := h.Quantile(q, lo, hi)
		if got < prev {
			t.Errorf("p%.0f = %v below the previous %v", q*100, got, prev)
		}
		prev = got
	}
	if got := h.Quantile(-1, lo, hi); got != lo {
		t.Errorf("q < 0 gave %v, want the minimum %v", got, lo)
	}
	if got := h.Quantile(2, lo, hi); got != hi {
		t.Errorf("q > 1 gave %v, want the maximum %v", got, hi)
	}
}

// Verifies: ANL-010.
// Adding two histograms is adding their counts, which is what folding
// rollups into a coarser granularity does, and a short row adds what it has.
func TestHistogramAdd_ANL_010(t *testing.T) {
	t.Parallel()
	a, _, _ := observe([]float64{1, 1, 100})
	b, _, _ := observe([]float64{1, 70000})
	a.Add(b[:])
	if a.Total() != 5 || a[1] != 3 || a[15] != 1 {
		t.Errorf("sum = %v", a)
	}
	a.Add([]int64{10})
	if a[0] != 10 || a.Total() != 15 {
		t.Errorf("a short row: %v", a)
	}
}

// Verifies: ANL-010.
// A granularity has a width, day buckets are 24 hours, and anything else is
// not a granularity.
func TestGranularity_ANL_010(t *testing.T) {
	t.Parallel()
	for g, w := range map[analytics.Granularity]string{analytics.FiveMinutes: "5m0s", analytics.Hour: "1h0m0s", analytics.Day: "24h0m0s"} {
		if !g.Valid() || g.Width().String() != w {
			t.Errorf("%s: valid=%v width=%v", g, g.Valid(), g.Width())
		}
	}
	for _, g := range []analytics.Granularity{"", "1w", "5M"} {
		if g.Valid() {
			t.Errorf("%q is valid", g)
		}
	}
}
