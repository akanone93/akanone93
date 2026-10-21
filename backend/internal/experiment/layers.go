// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"math"
	"slices"

	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// span is a half-open range [start, end) of a layer's buckets, held by the
// experiment key.
type span struct {
	start, end uint32
	key        string
}

// shareBP converts a share of a layer, in percent, to basis points. A share
// is above 0 and at most 100, with at most two decimals.
func shareBP(percent float64) (uint32, error) {
	bp := math.Round(percent * 100)
	if percent <= 0 || percent > 100 || math.Abs(percent*100-bp) > weightEpsilon {
		return 0, plxerr.New(plxerr.OutOfRange, "a layer share is above 0 and at most 100 percent, with at most two decimals, not %v", percent)
	}
	return uint32(bp), nil //nolint:gosec // bounded by the check above
}

// allocate returns the first free range of size buckets in a layer whose
// running experiments hold the occupied ranges, or PLX-8200 when there is
// none: mutually exclusive experiments never share a bucket (ABT-004). A
// running experiment keeps its range, so a later start never moves the
// subjects of an earlier one.
func allocate(layer string, occupied []span, size uint32) (span, error) {
	sorted := slices.Clone(occupied)
	slices.SortFunc(sorted, func(a, b span) int { return int(a.start) - int(b.start) })
	next := uint32(0)
	for _, o := range sorted {
		if o.start >= next && o.start-next >= size {
			break
		}
		next = max(next, o.end)
	}
	if next+size > controldoc.BucketCount {
		return span{}, overlap(layer, sorted, size)
	}
	return span{start: next, end: next + size}, nil
}

// overlap is the refusal of a start that finds no room in the layer.
func overlap(layer string, held []span, size uint32) error {
	var used uint32
	keys := make([]string, 0, len(held))
	for _, h := range held {
		used += h.end - h.start
		keys = append(keys, h.key)
	}
	free := controldoc.BucketCount - used
	return plxerr.New(plxerr.ExperimentOverlapsLayer,
		"layer %q has %.2f%% of its traffic free in no single range of the %.2f%% asked for; running there: %v; stop one of them or ask for a smaller share",
		layer, float64(free)/100, float64(size)/100, keys)
}
