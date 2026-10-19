// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Postgres is the core implementation of Store.
type Postgres struct {
	db  *storage.DB
	now func() time.Time

	mu sync.Mutex
	// ensured are the months whose partition this process has made or
	// seen, so that a write does not ask the database every time.
	ensured map[time.Time]bool
}

var _ Store = (*Postgres)(nil)

// NewPostgres returns the PostgreSQL store. A nil clock uses time.Now.
func NewPostgres(db *storage.DB, now func() time.Time) *Postgres {
	if now == nil {
		now = time.Now
	}
	return &Postgres{db: db, now: now, ensured: map[time.Time]bool{}}
}

// month is the first instant of the UTC month of t.
func month(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// rollupWatermarkLag is how long after receipt an event is folded into the
// rollups, so that a transaction that committed late is not missed.
const rollupWatermarkLag = time.Minute

// Prepare creates the partitions from the previous month to monthsAhead
// months after now.
func (p *Postgres) Prepare(ctx context.Context, now time.Time, monthsAhead int) error {
	first := month(now).AddDate(0, -1, 0)
	for i := 0; i <= monthsAhead+1; i++ {
		if err := p.ensure(ctx, first.AddDate(0, i, 0)); err != nil {
			return err
		}
	}
	return nil
}

// ensure makes the partition of a month.
func (p *Postgres) ensure(ctx context.Context, m time.Time) error {
	m = month(m)
	p.mu.Lock()
	known := p.ensured[m]
	p.mu.Unlock()
	if known {
		return nil
	}
	err := p.db.InTx(ctx, storage.Tenant{}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := dbgen.New(tx).EnsureEventsPartition(ctx, pgtype.Date{Time: m, Valid: true})
		return err //nolint:wrapcheck // wrapped below
	})
	if err != nil {
		return fmt.Errorf("analytics: create the partition of %s: %w", m.Format("2006-01"), err)
	}
	p.mu.Lock()
	p.ensured[m] = true
	p.mu.Unlock()
	return nil
}

// WriteEvents stores a batch in one round trip.
func (p *Postgres) WriteEvents(ctx context.Context, b Batch) error {
	if len(b.Events) == 0 {
		return nil
	}
	months := map[time.Time]bool{}
	for _, e := range b.Events {
		months[month(e.Time)] = true
	}
	for m := range months {
		if err := p.ensure(ctx, m); err != nil {
			return err
		}
	}
	org, app, env, dev, err := batchIDs(b)
	if err != nil {
		return err
	}
	params := make([]dbgen.InsertTelemetryEventParams, 0, len(b.Events))
	for _, e := range b.Events {
		id, err := storage.UUID(e.ID)
		if err != nil {
			return plxerr.New(plxerr.InvalidFormat, "the event identifier is not valid")
		}
		fields := e.Fields
		if len(fields) == 0 {
			fields = []byte("{}")
		}
		params = append(params, dbgen.InsertTelemetryEventParams{
			ID: id, OrganizationID: org, AppID: app, EnvironmentID: env, DeviceID: dev, Name: e.Name,
			Time: storage.Timestamp(e.Time), ReleaseSequence: e.ReleaseSequence, PluginKey: e.PluginKey, Route: e.Route,
			Fields: fields, DeviceClass: b.DeviceClass, Locale: b.Locale, Category: e.Category,
		})
	}
	err = p.db.InTx(ctx, storage.Tenant{OrganizationID: b.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		results := dbgen.New(tx).InsertTelemetryEvent(ctx, params)
		var first error
		results.Exec(func(_ int, err error) {
			if err != nil && first == nil {
				first = err
			}
		})
		if err := results.Close(); err != nil && first == nil {
			first = err
		}
		return first
	})
	if err != nil {
		return fmt.Errorf("analytics: write events: %w", err)
	}
	return nil
}

func batchIDs(b Batch) (org, app, env, dev pgtype.UUID, err error) {
	for i, s := range []string{b.OrganizationID, b.AppID, b.EnvironmentID, b.DeviceID} {
		id, perr := storage.UUID(s)
		if perr != nil {
			return org, app, env, dev, plxerr.New(plxerr.InvalidFormat, "an identifier of the batch is not valid")
		}
		switch i {
		case 0:
			org = id
		case 1:
			app = id
		case 2:
			env = id
		default:
			dev = id
		}
	}
	return org, app, env, dev, nil
}

// rkey identifies a rollup row.
type rkey struct {
	app, env                    pgtype.UUID
	name                        string
	bucket                      time.Time
	release                     int64
	plugin, page, class, locale string
}

// acc accumulates a rollup row.
type acc struct {
	events, valueCount int64
	sum, min, max      float64
	hist               Histogram
}

// add folds a partial row in.
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

// Rollup folds events received up to until into the rollups.
func (p *Postgres) Rollup(ctx context.Context, org string, until time.Time) error {
	until = until.Add(-rollupWatermarkLag)
	err := p.db.InTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		orgID := storage.MustUUID(org)
		after, err := q.GetWatermark(ctx, orgID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			after = storage.Timestamp(time.Unix(0, 0))
		case err != nil:
			return err //nolint:wrapcheck // wrapped below
		}
		if !until.After(storage.Time(after)) {
			return nil
		}
		touched, err := q.TouchedBuckets(ctx, dbgen.TouchedBucketsParams{OrganizationID: orgID, After: after, Until: storage.Timestamp(until)})
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		for _, r := range ranges(touched, FiveMinutes.Width(), time.Hour) {
			if err := rollupRange(ctx, q, orgID, r[0], r[1]); err != nil {
				return err
			}
		}
		return q.SetWatermark(ctx, dbgen.SetWatermarkParams{OrganizationID: orgID, RolledUntil: storage.Timestamp(until)}) //nolint:wrapcheck // wrapped below
	})
	if err != nil {
		return fmt.Errorf("analytics: roll up: %w", err)
	}
	return nil
}

// ranges merges sorted bucket starts into [from, to) ranges, joining
// buckets that are at most gap apart.
func ranges(buckets []pgtype.Timestamptz, width, gap time.Duration) [][2]time.Time {
	var out [][2]time.Time
	for _, b := range buckets {
		t := storage.Time(b)
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
func rollupRange(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, from, to time.Time) error {
	rows, err := q.AggregateRawEvents(ctx, dbgen.AggregateRawEventsParams{
		OrganizationID: org, FromTime: storage.Timestamp(from), ToTime: storage.Timestamp(to),
	})
	if err != nil {
		return err //nolint:wrapcheck // wrapped by the caller
	}
	groups := map[rkey]*acc{}
	for _, r := range rows {
		k := rkey{
			app: r.AppID, env: r.EnvironmentID, name: r.Name, bucket: storage.Time(r.Bucket), release: r.ReleaseSequence,
			plugin: r.PluginKey, page: r.Page, class: r.DeviceClass, locale: r.Locale,
		}
		a := groups[k]
		if a == nil {
			a = &acc{}
			groups[k] = a
		}
		a.add(r.Events, r.ValueCount, r.ValueSum, r.ValueMin, r.ValueMax)
		if r.HistBucket >= 0 && int(r.HistBucket) < Buckets {
			a.hist[r.HistBucket] += r.ValueCount
		}
	}
	if err := replace(ctx, q, org, FiveMinutes, from, to, groups); err != nil {
		return err
	}
	return refold(ctx, q, org, from, to)
}

// replace deletes the rollups of a granularity in [from, to) and writes the
// groups, in a fixed order.
func replace(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, g Granularity, from, to time.Time, groups map[rkey]*acc) error {
	if err := q.DeleteRollups(ctx, dbgen.DeleteRollupsParams{
		OrganizationID: org, Granularity: string(g), FromTime: storage.Timestamp(from), ToTime: storage.Timestamp(to),
	}); err != nil {
		return err //nolint:wrapcheck // wrapped by the caller
	}
	keys := make([]rkey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, compareKeys)
	for _, k := range keys {
		a := groups[k]
		if err := q.UpsertRollup(ctx, dbgen.UpsertRollupParams{
			OrganizationID: org, AppID: k.app, EnvironmentID: k.env, Granularity: string(g), EventName: k.name,
			Bucket: storage.Timestamp(k.bucket), ReleaseSequence: k.release, PluginKey: k.plugin, Page: k.page,
			DeviceClass: k.class, Locale: k.locale, Events: a.events, ValueCount: a.valueCount, ValueSum: a.sum,
			ValueMin: a.min, ValueMax: a.max, Histogram: slices.Clone(a.hist[:]),
		}); err != nil {
			return err //nolint:wrapcheck // wrapped by the caller
		}
	}
	return nil
}

func compareKeys(a, b rkey) int {
	if c := a.bucket.Compare(b.bucket); c != 0 {
		return c
	}
	if a.name != b.name {
		return cmp.Compare(a.name, b.name)
	}
	return compareRest(a, b)
}

func compareRest(a, b rkey) int {
	switch {
	case a.release != b.release:
		return cmp.Compare(a.release, b.release)
	case a.plugin != b.plugin:
		return cmp.Compare(a.plugin, b.plugin)
	case a.page != b.page:
		return cmp.Compare(a.page, b.page)
	case a.class != b.class:
		return cmp.Compare(a.class, b.class)
	case a.locale != b.locale:
		return cmp.Compare(a.locale, b.locale)
	case a.app != b.app:
		return slices.Compare(a.app.Bytes[:], b.app.Bytes[:])
	}
	return slices.Compare(a.env.Bytes[:], b.env.Bytes[:])
}

// refold recomputes the hourly rollups of the hours, and the daily rollups of
// the days, that [from, to) touches, from the rows one level below.
func refold(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, from, to time.Time) error {
	hFrom, hTo := from.Truncate(time.Hour), ceil(to, time.Hour)
	if err := fold(ctx, q, org, FiveMinutes, Hour, hFrom, hTo); err != nil {
		return err
	}
	dFrom, dTo := day(from), ceilDay(to)
	return fold(ctx, q, org, Hour, Day, dFrom, dTo)
}

func ceil(t time.Time, d time.Duration) time.Time {
	f := t.Truncate(d)
	if f.Equal(t) {
		return t
	}
	return f.Add(d)
}

func day(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func ceilDay(t time.Time) time.Time {
	d := day(t)
	if d.Equal(t.UTC()) {
		return d
	}
	return d.AddDate(0, 0, 1)
}

// fold sums the rollups of one granularity in [from, to) into the next.
func fold(ctx context.Context, q *dbgen.Queries, org pgtype.UUID, from, to Granularity, lo, hi time.Time) error {
	rows, err := q.ListRollupsInWindow(ctx, dbgen.ListRollupsInWindowParams{
		OrganizationID: org, Granularity: string(from), FromTime: storage.Timestamp(lo), ToTime: storage.Timestamp(hi),
	})
	if err != nil {
		return err //nolint:wrapcheck // wrapped by the caller
	}
	groups := map[rkey]*acc{}
	for _, r := range rows {
		b := storage.Time(r.Bucket)
		if to == Hour {
			b = b.Truncate(time.Hour)
		} else {
			b = day(b)
		}
		k := rkey{
			app: r.AppID, env: r.EnvironmentID, name: r.EventName, bucket: b, release: r.ReleaseSequence,
			plugin: r.PluginKey, page: r.Page, class: r.DeviceClass, locale: r.Locale,
		}
		a := groups[k]
		if a == nil {
			a = &acc{}
			groups[k] = a
		}
		a.add(r.Events, r.ValueCount, r.ValueSum, r.ValueMin, r.ValueMax)
		a.hist.Add(r.Histogram)
	}
	return replace(ctx, q, org, to, lo, hi, groups)
}

// Query reads and groups rollups.
func (p *Postgres) Query(ctx context.Context, org string, q Query) (Series, error) {
	if !q.Granularity.Valid() {
		return Series{}, plxerr.New(plxerr.InvalidEnumValue, "%q is not a granularity; use 5m, 1h or 1d", q.Granularity)
	}
	app, err := storage.UUID(q.AppID)
	if err != nil {
		return Series{}, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	var env pgtype.UUID
	if q.EnvironmentID != "" {
		if env, err = storage.UUID(q.EnvironmentID); err != nil {
			return Series{}, plxerr.New(plxerr.InvalidFormat, "the environment identifier is not valid")
		}
	}
	var rows []dbgen.AnalyticsRollup
	err = p.db.InTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = dbgen.New(tx).QueryRollups(ctx, dbgen.QueryRollupsParams{
			AppID: app, Granularity: string(q.Granularity), EventName: q.Metric, EnvironmentID: env,
			FromTime: storage.Timestamp(q.From), ToTime: storage.Timestamp(q.To), RowLimit: int32(q.Limit + 1), //nolint:gosec // bounded by api.pageSize
		})
		return err //nolint:wrapcheck // wrapped below
	})
	if err != nil {
		return Series{}, fmt.Errorf("analytics: query: %w", err)
	}
	series := Series{}
	if len(rows) > q.Limit {
		// The last bucket may be cut off; leave it for the next call.
		last := rows[len(rows)-1].Bucket
		cut := len(rows)
		for cut > 0 && rows[cut-1].Bucket.Time.Equal(last.Time) {
			cut--
		}
		if cut == 0 {
			return Series{}, plxerr.New(plxerr.LimitExceeded, "one bucket of %s has more than %d rows; group by fewer dimensions or filter the environment", q.Metric, q.Limit)
		}
		series.Truncated, series.NextFrom, rows = true, last.Time, rows[:cut]
	}
	series.Points = group(rows, q)
	return series, nil
}

// Window returns every rollup in [from, to), ungrouped.
func (p *Postgres) Window(ctx context.Context, org string, g Granularity, from, to time.Time) ([]Point, error) {
	var rows []dbgen.AnalyticsRollup
	err := p.db.InTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = dbgen.New(tx).ListRollupsInWindow(ctx, dbgen.ListRollupsInWindowParams{
			OrganizationID: storage.MustUUID(org), Granularity: string(g), FromTime: storage.Timestamp(from), ToTime: storage.Timestamp(to),
		})
		return err //nolint:wrapcheck // wrapped below
	})
	if err != nil {
		return nil, fmt.Errorf("analytics: read rollups: %w", err)
	}
	all := []Dimension{ByEnvironment, ByRelease, ByPlugin, ByPage, ByDeviceClass, ByLocale}
	pts := groupBy(rows, all, true)
	return pts, nil
}

func group(rows []dbgen.AnalyticsRollup, q Query) []Point {
	return groupBy(rows, q.GroupBy, false)
}

// gkey is a point's identity while grouping.
type gkey struct {
	app    pgtype.UUID
	name   string
	gran   string
	bucket time.Time
	group  Group
}

// groupBy sums rows that agree on the kept dimensions. withApp keeps the
// app apart too, for exports across apps.
func groupBy(rows []dbgen.AnalyticsRollup, keep []Dimension, withApp bool) []Point {
	type bucketed struct {
		acc acc
		key gkey
	}
	byKey := map[gkey]*bucketed{}
	for _, r := range rows {
		g := Group{}
		for _, d := range keep {
			switch d {
			case ByEnvironment:
				g.EnvironmentID = storage.ID(r.EnvironmentID)
			case ByRelease:
				g.Release = r.ReleaseSequence
			case ByPlugin:
				g.Plugin = r.PluginKey
			case ByPage:
				g.Page = r.Page
			case ByDeviceClass:
				g.DeviceClass = r.DeviceClass
			case ByLocale:
				g.Locale = r.Locale
			}
		}
		k := gkey{name: r.EventName, gran: r.Granularity, bucket: storage.Time(r.Bucket), group: g}
		if withApp {
			k.app = r.AppID
		}
		b := byKey[k]
		if b == nil {
			b = &bucketed{key: k}
			byKey[k] = b
		}
		b.acc.add(r.Events, r.ValueCount, r.ValueSum, r.ValueMin, r.ValueMax)
		b.acc.hist.Add(r.Histogram)
	}
	out := make([]Point, 0, len(byKey))
	for k, b := range byKey {
		a := b.acc
		pt := Point{
			AppID: storage.ID(k.app), EventName: k.name, Granularity: Granularity(k.gran), Bucket: k.bucket, Group: k.group,
			Events: a.events, ValueCount: a.valueCount, Sum: a.sum, Min: a.min, Max: a.max, Histogram: a.hist,
		}
		pt.P50 = a.hist.Quantile(0.5, a.min, a.max)
		pt.P90 = a.hist.Quantile(0.9, a.min, a.max)
		pt.P99 = a.hist.Quantile(0.99, a.min, a.max)
		out = append(out, pt)
	}
	slices.SortFunc(out, func(a, b Point) int {
		if c := a.Bucket.Compare(b.Bucket); c != 0 {
			return c
		}
		if a.EventName != b.EventName {
			return cmp.Compare(a.EventName, b.EventName)
		}
		return compareGroups(a.AppID, b.AppID, a.Group, b.Group)
	})
	return out
}

func compareGroups(appA, appB string, a, b Group) int {
	switch {
	case appA != appB:
		return cmp.Compare(appA, appB)
	case a.EnvironmentID != b.EnvironmentID:
		return cmp.Compare(a.EnvironmentID, b.EnvironmentID)
	case a.Release != b.Release:
		return cmp.Compare(a.Release, b.Release)
	case a.Plugin != b.Plugin:
		return cmp.Compare(a.Plugin, b.Plugin)
	case a.Page != b.Page:
		return cmp.Compare(a.Page, b.Page)
	case a.DeviceClass != b.DeviceClass:
		return cmp.Compare(a.DeviceClass, b.DeviceClass)
	}
	return cmp.Compare(a.Locale, b.Locale)
}

// Purge deletes raw events and rollups past their cut-offs.
func (p *Postgres) Purge(ctx context.Context, org string, pu Purge) (Purged, error) {
	var out Purged
	err := p.db.InTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		orgID := storage.MustUUID(org)
		var err error
		if !pu.RawBefore.IsZero() {
			if out.Raw, err = q.DeleteRawEventsBefore(ctx, dbgen.DeleteRawEventsBeforeParams{OrganizationID: orgID, Time: storage.Timestamp(pu.RawBefore)}); err != nil {
				return err //nolint:wrapcheck // wrapped below
			}
		}
		if !pu.RollupsBefore.IsZero() {
			if out.Rollups, err = q.DeleteRollupsBefore(ctx, dbgen.DeleteRollupsBeforeParams{OrganizationID: orgID, Bucket: storage.Timestamp(pu.RollupsBefore)}); err != nil {
				return err //nolint:wrapcheck // wrapped below
			}
		}
		return nil
	})
	if err != nil {
		return Purged{}, fmt.Errorf("analytics: purge: %w", err)
	}
	return out, nil
}

// maxPartitionAge is how many months back Compact looks for partitions.
const maxPartitionAge = 120

// Compact drops the partitions that end at or before the time.
func (p *Postgres) Compact(ctx context.Context, before time.Time) (int, error) {
	last := month(before)
	if last.AddDate(0, 1, 0).After(before.UTC()) {
		last = last.AddDate(0, -1, 0)
	}
	dropped := 0
	for i := 0; i < maxPartitionAge; i++ {
		m := last.AddDate(0, -i, 0)
		var did bool
		err := p.db.InTx(ctx, storage.Tenant{}, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			did, err = dbgen.New(tx).DropEventsPartition(ctx, pgtype.Date{Time: m, Valid: true})
			return err //nolint:wrapcheck // wrapped below
		})
		if err != nil {
			return dropped, fmt.Errorf("analytics: drop the partition of %s: %w", m.Format("2006-01"), err)
		}
		if did {
			dropped++
			p.mu.Lock()
			delete(p.ensured, m)
			p.mu.Unlock()
		}
	}
	return dropped, nil
}
