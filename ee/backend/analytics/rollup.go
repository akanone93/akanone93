// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	ext "github.com/nightCode42/plux3/backend/ext/analytics"
)

// rkey identifies a rollup row apart from its organisation and granularity.
// The bucket is in microseconds since the epoch, so that the key does not
// depend on a time zone pointer.
type rkey struct {
	app, env, name              string
	bucket                      int64
	release                     int64
	plugin, page, class, locale string
}

// acc accumulates a rollup row.
type acc struct {
	events, valueCount int64
	sum, min, max      float64
	hist               ext.Histogram
}

// add folds a partial row in. Rows without a value leave the extremes
// alone.
func (a *acc) add(events, valueCount int64, sum, minV, maxV float64) {
	a.events += events
	if valueCount == 0 {
		return
	}
	if a.valueCount == 0 || minV < a.min {
		a.min = minV
	}
	if a.valueCount == 0 || maxV > a.max {
		a.max = maxV
	}
	a.valueCount += valueCount
	a.sum += sum
}

// Rollup folds the events received before until, and not yet folded, into
// the rollups of an organisation. The work is idempotent: a run that stops
// before it records its watermark is repeated by the next one and writes the
// same rows again.
func (s *Store) Rollup(ctx context.Context, org string, until time.Time) error {
	org, err := canonicalUUID(org)
	if err != nil {
		return fmt.Errorf("analytics: roll up: the organisation: %w", err)
	}
	until = until.Add(-rollupWatermarkLag)
	after, err := s.watermark(ctx, org)
	if err != nil {
		return err
	}
	if !until.After(after) {
		return nil
	}
	touched, err := s.touchedBuckets(ctx, org, after, until)
	if err != nil {
		return err
	}
	for _, r := range ranges(touched, ext.FiveMinutes.Width(), time.Hour) {
		if err := s.rollupRange(ctx, org, r[0], r[1]); err != nil {
			return err
		}
	}
	return s.setWatermark(ctx, org, until)
}

// watermark returns the time an organisation is rolled up to, the epoch for
// an organisation never rolled up.
func (s *Store) watermark(ctx context.Context, org string) (time.Time, error) {
	at := time.Unix(0, 0).UTC()
	err := s.collect(ctx, queryWatermark, []any{param("org", org)}, func(r rowScanner) error {
		return r.Scan(&at)
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("analytics: roll up: read the watermark: %w", err)
	}
	return at.UTC(), nil
}

// setWatermark records how far an organisation is rolled up.
func (s *Store) setWatermark(ctx context.Context, org string, until time.Time) error {
	row := []any{org, until.UTC(), s.nextVersion()}
	if err := s.insert(ctx, watermarksTable, []string{"organization_id", "rolled_until", "version"}, [][]any{row}); err != nil {
		return fmt.Errorf("analytics: roll up: record the watermark: %w", err)
	}
	return nil
}

// touchedBuckets lists the five-minute buckets that events received in
// (after, until] fall in, oldest first.
func (s *Store) touchedBuckets(ctx context.Context, org string, after, until time.Time) ([]time.Time, error) {
	args := []any{param("org", org), timeParam("after", after), timeParam("until", until)}
	var out []time.Time
	err := s.collect(ctx, queryTouchedBuckets, args, func(r rowScanner) error {
		var t time.Time
		if err := r.Scan(&t); err != nil {
			return err //nolint:wrapcheck // wrapped by collect
		}
		out = append(out, t.UTC())
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("analytics: roll up: find the touched buckets: %w", err)
	}
	return out, nil
}

// ranges merges sorted bucket starts into [from, to) ranges, joining
// buckets that are at most gap apart.
func ranges(buckets []time.Time, width, gap time.Duration) [][2]time.Time {
	var out [][2]time.Time
	for _, t := range buckets {
		if n := len(out); n > 0 && t.Sub(out[n-1][1]) <= gap {
			out[n-1][1] = t.Add(width)
			continue
		}
		out = append(out, [2]time.Time{t, t.Add(width)})
	}
	return out
}

// rollupRange recomputes the five-minute rollups of [from, to) from the raw
// events and refolds the hours and days they touch.
func (s *Store) rollupRange(ctx context.Context, org string, from, to time.Time) error {
	groups, err := s.aggregate(ctx, org, from, to)
	if err != nil {
		return err
	}
	if err := s.writeRollups(ctx, org, ext.FiveMinutes, groups); err != nil {
		return err
	}
	hFrom, hTo := from.Truncate(time.Hour), ceil(to, time.Hour)
	if err := s.fold(ctx, org, ext.FiveMinutes, ext.Hour, hFrom, hTo); err != nil {
		return err
	}
	return s.fold(ctx, org, ext.Hour, ext.Day, day(from), ceilDay(to))
}

// aggregate groups the raw events of [from, to) into five-minute rows.
func (s *Store) aggregate(ctx context.Context, org string, from, to time.Time) (map[rkey]*acc, error) {
	args := []any{param("org", org), timeParam("from", from), timeParam("to", to)}
	groups := map[rkey]*acc{}
	err := s.collect(ctx, s.aggregateRaw, args, func(r rowScanner) error {
		var (
			bucket             time.Time
			k                  rkey
			hist               int32
			events, valueCount uint64
			sum, minV, maxV    float64
		)
		if err := r.Scan(&bucket, &k.app, &k.env, &k.name, &k.release, &k.plugin, &k.page, &k.class, &k.locale,
			&hist, &events, &valueCount, &sum, &minV, &maxV); err != nil {
			return err //nolint:wrapcheck // wrapped by collect
		}
		k.bucket = bucket.UnixMicro()
		a := groups[k]
		if a == nil {
			a = &acc{}
			groups[k] = a
		}
		a.add(int64(events), int64(valueCount), sum, minV, maxV) //nolint:gosec // row counts
		if hist >= 0 && int(hist) < ext.Buckets {
			a.hist[hist] += int64(valueCount) //nolint:gosec // a row count
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("analytics: roll up: aggregate the raw events: %w", err)
	}
	return groups, nil
}

// writeRollups inserts the groups of one granularity, in a fixed order.
// Rows that exist are replaced: the new rows carry a higher version.
func (s *Store) writeRollups(ctx context.Context, org string, g ext.Granularity, groups map[rkey]*acc) error {
	keys := make([]rkey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, compareKeys)
	version := s.nextVersion()
	rows := make([][]any, 0, len(keys))
	for _, k := range keys {
		a := groups[k]
		rows = append(rows, []any{
			org, k.app, k.env, string(g), k.name, time.UnixMicro(k.bucket).UTC(), k.release, k.plugin, k.page, k.class, k.locale,
			a.events, a.valueCount, a.sum, a.min, a.max, slices.Clone(a.hist[:]), version,
		})
	}
	if err := s.insert(ctx, rollupsTable, rollupColumns(), rows); err != nil {
		return fmt.Errorf("analytics: roll up: write the %s rollups: %w", g, err)
	}
	return nil
}

// compareKeys orders rollup rows by bucket, name and then dimensions.
func compareKeys(a, b rkey) int {
	return cmp.Or(
		cmp.Compare(a.bucket, b.bucket), cmp.Compare(a.name, b.name), cmp.Compare(a.release, b.release),
		cmp.Compare(a.plugin, b.plugin), cmp.Compare(a.page, b.page), cmp.Compare(a.class, b.class),
		cmp.Compare(a.locale, b.locale), cmp.Compare(a.app, b.app), cmp.Compare(a.env, b.env),
	)
}

// fold sums the rollups of one granularity in [lo, hi) into the next.
func (s *Store) fold(ctx context.Context, org string, from, to ext.Granularity, lo, hi time.Time) error {
	rows, err := s.rollupRows(ctx, org, from, lo, hi)
	if err != nil {
		return err
	}
	groups := map[rkey]*acc{}
	for _, r := range rows {
		b := r.bucket
		if to == ext.Hour {
			b = b.Truncate(time.Hour)
		} else {
			b = day(b)
		}
		k := rkey{
			app: r.app, env: r.env, name: r.name, bucket: b.UnixMicro(), release: r.release,
			plugin: r.plugin, page: r.page, class: r.class, locale: r.locale,
		}
		a := groups[k]
		if a == nil {
			a = &acc{}
			groups[k] = a
		}
		a.add(r.events, r.valueCount, r.sum, r.min, r.max)
		a.hist.Add(r.hist)
	}
	return s.writeRollups(ctx, org, to, groups)
}

// ceil rounds t up to a multiple of d.
func ceil(t time.Time, d time.Duration) time.Time {
	f := t.Truncate(d)
	if f.Equal(t) {
		return t
	}
	return f.Add(d)
}

// day is the start of the UTC day of t.
func day(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// ceilDay rounds t up to the start of a UTC day.
func ceilDay(t time.Time) time.Time {
	d := day(t)
	if d.Equal(t.UTC()) {
		return d
	}
	return d.AddDate(0, 0, 1)
}
