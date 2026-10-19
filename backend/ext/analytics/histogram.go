// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package analytics

import "math"

// Bounds are the upper edges, in milliseconds, of the histogram buckets
// that stand in for percentile sketches (ANL-010): bucket i holds the
// values at or above Bounds[i-1] and below Bounds[i]; bucket 0 is below the
// first bound and the last bucket is at or above the last. The same bounds
// are in the database function plux_value_bucket (migration 0027).
var Bounds = [...]float64{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 30000, 60000}

// Buckets is the number of histogram buckets.
const Buckets = len(Bounds) + 1

// Histogram counts values by bucket.
type Histogram [Buckets]int64

// BucketOf returns the bucket of a value.
func BucketOf(v float64) int {
	for i, b := range Bounds {
		if v < b {
			return i
		}
	}
	return len(Bounds)
}

// Add adds another histogram's counts.
func (h *Histogram) Add(o []int64) {
	for i := range h {
		if i < len(o) {
			h[i] += o[i]
		}
	}
}

// Total is the number of values counted.
func (h *Histogram) Total() int64 {
	var n int64
	for _, c := range h {
		n += c
	}
	return n
}

// Quantile estimates the q-th quantile (0 to 1) of the counted values by
// linear interpolation inside the bucket it falls in. The first bucket
// starts at min and the last ends at max, which the rollup keeps exactly, so
// the estimate never leaves the observed range. The error is bounded by the
// width of one bucket. An empty histogram gives 0.
func (h *Histogram) Quantile(q, minValue, maxValue float64) float64 {
	total := h.Total()
	if total == 0 {
		return 0
	}
	q = math.Min(math.Max(q, 0), 1)
	rank := q * float64(total)
	var seen float64
	for i, c := range h {
		if c == 0 {
			continue
		}
		next := seen + float64(c)
		if rank <= next {
			lo, hi := bucketEdges(i, minValue, maxValue)
			frac := (rank - seen) / float64(c)
			return math.Min(math.Max(lo+frac*(hi-lo), minValue), maxValue)
		}
		seen = next
	}
	return maxValue
}

// bucketEdges returns the edges of bucket i, clamped to the observed range.
func bucketEdges(i int, minValue, maxValue float64) (lo, hi float64) {
	lo, hi = minValue, maxValue
	if i > 0 {
		lo = math.Max(Bounds[i-1], minValue)
	}
	if i < len(Bounds) {
		hi = math.Min(Bounds[i], maxValue)
	}
	if hi < lo {
		hi = lo
	}
	return lo, hi
}
