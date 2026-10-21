// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	ext "github.com/nightCode42/plux3/backend/ext/analytics"
)

// rollupRow is a rollup as stored.
type rollupRow struct {
	app, env, gran, name        string
	bucket                      time.Time
	release                     int64
	plugin, page, class, locale string
	events, valueCount          int64
	sum, min, max               float64
	hist                        []int64
}

// scanRollup reads a row selected with rollupSelect.
func scanRollup(r rowScanner) (rollupRow, error) {
	var row rollupRow
	err := r.Scan(&row.app, &row.env, &row.gran, &row.name, &row.bucket, &row.release, &row.plugin, &row.page,
		&row.class, &row.locale, &row.events, &row.valueCount, &row.sum, &row.min, &row.max, &row.hist)
	row.bucket = row.bucket.UTC()
	return row, err
}

// readRollups runs a rollup query and returns its rows.
func (s *Store) readRollups(ctx context.Context, query string, args []any) ([]rollupRow, error) {
	var rows []rollupRow
	err := s.collect(ctx, query, args, func(r rowScanner) error {
		row, err := scanRollup(r)
		if err != nil {
			return err
		}
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("analytics: read rollups: %w", err)
	}
	return rows, nil
}

// rollupRows reads the rollups of one granularity in [lo, hi).
func (s *Store) rollupRows(ctx context.Context, org string, g ext.Granularity, lo, hi time.Time) ([]rollupRow, error) {
	args := []any{param("org", org), param("granularity", string(g)), timeParam("from", lo), timeParam("to", hi)}
	return s.readRollups(ctx, queryRollupWindow, args)
}

// Window returns every rollup of an organisation in [from, to), ungrouped.
func (s *Store) Window(ctx context.Context, org string, g ext.Granularity, from, to time.Time) ([]ext.Point, error) {
	org, err := canonicalUUID(org)
	if err != nil {
		return nil, fmt.Errorf("analytics: read rollups: the organisation: %w", err)
	}
	if !g.Valid() {
		return nil, errors.New("analytics: read rollups: the granularity is not 5m, 1h or 1d")
	}
	rows, err := s.rollupRows(ctx, org, g, from, to)
	if err != nil {
		return nil, err
	}
	return groupBy(rows, ext.Dimensions(), true), nil
}

// Query reads and groups the rollups of one app and metric.
func (s *Store) Query(ctx context.Context, org string, q ext.Query) (ext.Series, error) {
	if !q.Granularity.Valid() {
		return ext.Series{}, fmt.Errorf("analytics: query: %q is not a granularity; use 5m, 1h or 1d", q.Granularity)
	}
	query, args, err := seriesQuery(org, q)
	if err != nil {
		return ext.Series{}, err
	}
	rows, err := s.readRollups(ctx, query, args)
	if err != nil {
		return ext.Series{}, err
	}
	series := ext.Series{}
	if len(rows) > q.Limit {
		// The last bucket may be cut off; leave it for the next call.
		last := rows[len(rows)-1].bucket
		cut := len(rows)
		for cut > 0 && rows[cut-1].bucket.Equal(last) {
			cut--
		}
		if cut == 0 {
			return ext.Series{}, fmt.Errorf("analytics: query: one bucket of %s has more than %d rows; group by fewer dimensions or filter the environment", q.Metric, q.Limit)
		}
		series.Truncated, series.NextFrom, rows = true, last, rows[:cut]
	}
	series.Points = groupBy(rows, q.GroupBy, false)
	return series, nil
}

// seriesQuery builds the query and parameters of a series. It reads one row
// more than the limit, to tell that the limit was reached.
func seriesQuery(org string, q ext.Query) (string, []any, error) {
	org, err := canonicalUUID(org)
	if err != nil {
		return "", nil, fmt.Errorf("analytics: query: the organisation: %w", err)
	}
	app, err := canonicalUUID(q.AppID)
	if err != nil {
		return "", nil, fmt.Errorf("analytics: query: the app: %w", err)
	}
	query := queryRollupSeries
	args := []any{
		param("org", org), param("app", app), param("granularity", string(q.Granularity)),
		param("metric", q.Metric), timeParam("from", q.From), timeParam("to", q.To),
	}
	if q.EnvironmentID != "" {
		env, err := canonicalUUID(q.EnvironmentID)
		if err != nil {
			return "", nil, fmt.Errorf("analytics: query: the environment: %w", err)
		}
		query += seriesInEnvironment
		args = append(args, param("env", env))
	}
	query += seriesOrder
	args = append(args, param("row_limit", uint64(max(q.Limit, 0))+1))
	return query, args, nil
}

// gkey is a point's identity while grouping.
type gkey struct {
	app    string
	name   string
	gran   string
	bucket int64
	group  ext.Group
}

// groupBy sums rows that agree on the kept dimensions. withApp keeps the app
// apart too, for exports across apps. The points come ordered by bucket,
// name and group.
func groupBy(rows []rollupRow, keep []ext.Dimension, withApp bool) []ext.Point {
	byKey := map[gkey]*acc{}
	for _, r := range rows {
		k := gkey{name: r.name, gran: r.gran, bucket: r.bucket.UnixMicro(), group: groupOf(r, keep)}
		if withApp {
			k.app = r.app
		}
		a := byKey[k]
		if a == nil {
			a = &acc{}
			byKey[k] = a
		}
		a.add(r.events, r.valueCount, r.sum, r.min, r.max)
		a.hist.Add(r.hist)
	}
	out := make([]ext.Point, 0, len(byKey))
	for k, a := range byKey {
		out = append(out, ext.Point{
			AppID: k.app, EventName: k.name, Granularity: ext.Granularity(k.gran), Bucket: time.UnixMicro(k.bucket).UTC(),
			Group: k.group, Events: a.events, ValueCount: a.valueCount, Sum: a.sum, Min: a.min, Max: a.max,
			P50: a.hist.Quantile(0.5, a.min, a.max), P90: a.hist.Quantile(0.9, a.min, a.max), P99: a.hist.Quantile(0.99, a.min, a.max),
			Histogram: a.hist,
		})
	}
	slices.SortFunc(out, comparePoints)
	return out
}

// groupOf keeps the dimensions of a row that the query groups by.
func groupOf(r rollupRow, keep []ext.Dimension) ext.Group {
	var g ext.Group
	for _, d := range keep {
		switch d {
		case ext.ByEnvironment:
			g.EnvironmentID = r.env
		case ext.ByRelease:
			g.Release = r.release
		case ext.ByPlugin:
			g.Plugin = r.plugin
		case ext.ByPage:
			g.Page = r.page
		case ext.ByDeviceClass:
			g.DeviceClass = r.class
		case ext.ByLocale:
			g.Locale = r.locale
		}
	}
	return g
}

// comparePoints orders points by bucket, name, app and group.
func comparePoints(a, b ext.Point) int {
	return cmp.Or(
		a.Bucket.Compare(b.Bucket), cmp.Compare(a.EventName, b.EventName), cmp.Compare(a.AppID, b.AppID),
		cmp.Compare(a.Group.EnvironmentID, b.Group.EnvironmentID), cmp.Compare(a.Group.Release, b.Group.Release),
		cmp.Compare(a.Group.Plugin, b.Group.Plugin), cmp.Compare(a.Group.Page, b.Group.Page),
		cmp.Compare(a.Group.DeviceClass, b.Group.DeviceClass), cmp.Compare(a.Group.Locale, b.Group.Locale),
	)
}
