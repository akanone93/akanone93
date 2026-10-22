// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	ext "github.com/nightCode42/plux3/backend/ext/analytics"
)

// call is a statement the store sent.
type call struct {
	query string
	args  []any
}

// namedArg returns the value bound to a name, or nil.
func (c call) namedArg(name string) any {
	for _, a := range c.args {
		switch v := a.(type) {
		case driver.NamedValue:
			if v.Name == name {
				return v.Value
			}
		case driver.NamedDateValue:
			if v.Name == name {
				return v.Value
			}
		}
	}
	return nil
}

// storedEvent is a row of the fake events table.
type storedEvent struct {
	row      []any
	received time.Time
}

// fakeConn is an in-memory stand-in for ClickHouse that answers the store's
// queries by interpreting their parameters. It checks the store's Go: the
// folding, versioning, grouping and paging. The SQL text itself runs only in
// the live test (PLUX_TEST_CLICKHOUSE_URL).
type fakeConn struct {
	calls     []call
	events    []storedEvent
	rollups   map[string][]any
	watermark map[string]time.Time
	batches   int
	closed    bool
}

var _ conn = (*fakeConn)(nil)

func newFakeConn() *fakeConn {
	return &fakeConn{rollups: map[string][]any{}, watermark: map[string]time.Time{}}
}

// Close records the close.
func (f *fakeConn) Close() error { f.closed = true; return nil }

// Exec records a statement and applies deletions.
func (f *fakeConn) Exec(_ context.Context, query string, args ...any) error {
	f.calls = append(f.calls, call{query, args})
	c := call{query, args}
	switch query {
	case deleteRaw:
		org, before := c.namedArg("org"), c.namedArg("before").(time.Time)
		f.events = slices.DeleteFunc(f.events, func(e storedEvent) bool {
			return e.row[0] == org && e.row[6].(time.Time).Before(before)
		})
	case deleteRollups:
		org, before := c.namedArg("org"), c.namedArg("before").(time.Time)
		for k, r := range f.rollups {
			if r[0] == org && r[5].(time.Time).Before(before) {
				delete(f.rollups, k)
			}
		}
	}
	return nil
}

// fakeBatch collects rows until Send.
type fakeBatch struct {
	driver.Batch
	conn    *fakeConn
	table   string
	rows    [][]any
	aborted bool
}

func (b *fakeBatch) Append(v ...any) error { b.rows = append(b.rows, v); return nil }
func (b *fakeBatch) Abort() error          { b.aborted = true; return nil }

// Send applies the rows to the fake tables.
func (b *fakeBatch) Send() error {
	f := b.conn
	f.batches++
	for _, r := range b.rows {
		switch b.table {
		case eventsTable:
			f.events = append(f.events, storedEvent{row: r, received: r[7].(time.Time)})
		case rollupsTable:
			key := fmt.Sprint(r[:11]...)
			if old, ok := f.rollups[key]; !ok || r[17].(uint64) > old[17].(uint64) {
				f.rollups[key] = r
			}
		case watermarksTable:
			f.watermark[r[0].(string)] = r[1].(time.Time)
		}
	}
	return nil
}

// PrepareBatch starts a batch for the table the INSERT names.
func (f *fakeConn) PrepareBatch(_ context.Context, query string, _ ...driver.PrepareBatchOption) (driver.Batch, error) {
	f.calls = append(f.calls, call{query: query})
	table := strings.Fields(query)[2]
	return &fakeBatch{conn: f, table: table}, nil
}

// fakeRows serves canned rows.
type fakeRows struct {
	driver.Rows
	rows [][]any
	at   int
}

func (r *fakeRows) Next() bool { r.at++; return r.at <= len(r.rows) }
func (*fakeRows) Close() error { return nil }
func (*fakeRows) Err() error   { return nil }

// Scan assigns the current row to the destinations, converting numbers.
func (r *fakeRows) Scan(dest ...any) error {
	row := r.rows[r.at-1]
	if len(dest) != len(row) {
		return fmt.Errorf("scan: %d destinations for %d columns", len(dest), len(row))
	}
	for i, d := range dest {
		dv := reflect.ValueOf(d).Elem()
		sv := reflect.ValueOf(row[i])
		if sv.Type().AssignableTo(dv.Type()) {
			dv.Set(sv)
		} else {
			dv.Set(sv.Convert(dv.Type()))
		}
	}
	return nil
}

// Query answers a store query from the fake tables.
func (f *fakeConn) Query(_ context.Context, query string, args ...any) (driver.Rows, error) {
	c := call{query, args}
	f.calls = append(f.calls, c)
	switch {
	case query == queryWatermark:
		if at, ok := f.watermark[c.namedArg("org").(string)]; ok {
			return &fakeRows{rows: [][]any{{at}}}, nil
		}
		return &fakeRows{}, nil
	case query == queryTouchedBuckets:
		return &fakeRows{rows: f.touched(c)}, nil
	case query == aggregateRawQuery():
		return &fakeRows{rows: f.aggregate(c)}, nil
	case query == queryRollupWindow:
		return &fakeRows{rows: f.rollupRows(c, "", "")}, nil
	case strings.HasPrefix(query, queryRollupSeries):
		return &fakeRows{rows: f.series(c)}, nil
	case query == queryEvents:
		return &fakeRows{rows: f.eventPage(c)}, nil
	case query == queryCountRaw:
		return &fakeRows{rows: [][]any{{uint64(len(f.dedupe(c.namedArg("org").(string), time.Time{}, c.namedArg("before").(time.Time))))}}}, nil
	case query == queryCountRollups:
		return &fakeRows{rows: [][]any{{nonNegative(f.countRollups(c))}}}, nil
	case query == queryPartitions:
		return &fakeRows{rows: f.partitions()}, nil
	}
	return nil, fmt.Errorf("fake: unexpected query %q", query)
}

// dedupe returns the events of an organisation in [from, to) once per
// (id, time), ordered by time and id.
func (f *fakeConn) dedupe(org string, from, to time.Time) [][]any {
	seen := map[string]bool{}
	var out [][]any
	for _, e := range f.events {
		t := e.row[6].(time.Time)
		key := fmt.Sprint(e.row[4], t.UnixMicro())
		if e.row[0] != org || t.Before(from) || !t.Before(to) || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e.row)
	}
	slices.SortFunc(out, func(a, b []any) int {
		if c := a[6].(time.Time).Compare(b[6].(time.Time)); c != 0 {
			return c
		}
		return strings.Compare(a[4].(string), b[4].(string))
	})
	return out
}

func (f *fakeConn) touched(c call) [][]any {
	org, after, until := c.namedArg("org"), c.namedArg("after").(time.Time), c.namedArg("until").(time.Time)
	set := map[int64]bool{}
	for _, e := range f.events {
		if e.row[0] == org && e.received.After(after) && !e.received.After(until) {
			set[e.row[6].(time.Time).Truncate(5*time.Minute).UnixMicro()] = true
		}
	}
	var keys []int64
	for k := range set {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	rows := make([][]any, len(keys))
	for i, k := range keys {
		rows[i] = []any{time.UnixMicro(k).UTC()}
	}
	return rows
}

// duration is the event's duration as the SQL reads it: a JSON number under
// build_ms for render_perf and duration_ms otherwise.
func duration(name, fields string) (float64, bool) {
	key := "duration_ms"
	if name == "render_perf" {
		key = "build_ms"
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(fields), &m) != nil {
		return 0, false
	}
	var v float64
	if json.Unmarshal(m[key], &v) != nil {
		return 0, false
	}
	return v, true
}

func (f *fakeConn) aggregate(c call) [][]any {
	type key struct {
		bucket                       int64
		app, env, name               string
		release                      int64
		plugin, route, class, locale string
		hist                         int32
	}
	type sums struct {
		events, values uint64
		sum, min, max  float64
	}
	groups := map[key]*sums{}
	for _, r := range f.dedupe(c.namedArg("org").(string), c.namedArg("from").(time.Time), c.namedArg("to").(time.Time)) {
		name := r[5].(string)
		k := key{
			bucket: r[6].(time.Time).Truncate(5 * time.Minute).UnixMicro(), app: r[1].(string), env: r[2].(string), name: name,
			release: r[8].(int64), plugin: r[9].(string), route: r[10].(string), class: r[12].(string), locale: r[13].(string), hist: -1,
		}
		v, ok := duration(name, r[11].(string))
		if ok {
			k.hist = bucketIndex(ext.BucketOf(v))
		}
		g := groups[k]
		if g == nil {
			g = &sums{}
			groups[k] = g
		}
		g.events++
		if ok {
			if g.values == 0 || v < g.min {
				g.min = v
			}
			if g.values == 0 || v > g.max {
				g.max = v
			}
			g.values++
			g.sum += v
		}
	}
	var rows [][]any
	for k, g := range groups {
		rows = append(rows, []any{
			time.UnixMicro(k.bucket).UTC(), k.app, k.env, k.name, k.release, k.plugin, k.route, k.class, k.locale,
			k.hist, g.events, g.values, g.sum, g.min, g.max,
		})
	}
	return rows
}

// rollupRows returns the stored rollups of the call's window as select rows;
// app and metric, when set, restrict them.
func (f *fakeConn) rollupRows(c call, app, metric string) [][]any {
	org, gran := c.namedArg("org"), c.namedArg("granularity")
	from, to := c.namedArg("from").(time.Time), c.namedArg("to").(time.Time)
	var rows [][]any
	for _, r := range f.rollups {
		b := r[5].(time.Time)
		if r[0] != org || r[3] != gran || b.Before(from) || !b.Before(to) {
			continue
		}
		if (app != "" && r[1] != app) || (metric != "" && r[4] != metric) {
			continue
		}
		// Select order: app, env, gran, name, bucket, release, plugin, page,
		// class, locale, events, value_count, sum, min, max, histogram.
		rows = append(rows, append([]any{r[1], r[2]}, r[3:17]...))
	}
	return rows
}

func (f *fakeConn) series(c call) [][]any {
	rows := f.rollupRows(c, c.namedArg("app").(string), c.namedArg("metric").(string))
	if env := c.namedArg("env"); env != nil {
		rows = slices.DeleteFunc(rows, func(r []any) bool { return r[1] != env })
	}
	slices.SortStableFunc(rows, func(a, b []any) int { return a[4].(time.Time).Compare(b[4].(time.Time)) })
	if limit := rowLimit(c); len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func (f *fakeConn) eventPage(c call) [][]any {
	all := f.dedupe(c.namedArg("org").(string), c.namedArg("from").(time.Time), c.namedArg("to").(time.Time))
	ct, cid := c.namedArg("cursor_time").(time.Time), c.namedArg("cursor_id").(string)
	var rows [][]any
	for _, r := range all {
		t := r[6].(time.Time)
		if t.Before(ct) || (t.Equal(ct) && r[4].(string) <= cid) {
			continue
		}
		// Select order: id, org, app, env, device, name, time, release,
		// plugin, route, category, class, locale, fields.
		rows = append(rows, []any{r[4], r[0], r[1], r[2], r[3], r[5], r[6], r[8], r[9], r[10], r[14], r[12], r[13], r[11]})
	}
	if limit := rowLimit(c); len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

// nonNegative converts a count to the unsigned type the driver reports.
func nonNegative(n int) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}

// bucketIndex converts a histogram bucket to the column type.
func bucketIndex(n int) int32 {
	if n < 0 || n > math.MaxInt32 {
		return 0
	}
	return int32(n)
}

// rowLimit returns the row_limit argument of the call, capped to the range
// of int.
func rowLimit(c call) int {
	limit := c.namedArg("row_limit").(uint64)
	if limit > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(limit)
}

func (f *fakeConn) countRollups(c call) int {
	n := 0
	for _, r := range f.rollups {
		if r[0] == c.namedArg("org") && r[5].(time.Time).Before(c.namedArg("before").(time.Time)) {
			n++
		}
	}
	return n
}

func (f *fakeConn) partitions() [][]any {
	set := map[string]bool{}
	for _, e := range f.events {
		set[e.row[6].(time.Time).Format("200601")] = true
	}
	var ids []string
	for id := range set {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	rows := make([][]any, len(ids))
	for i, id := range ids {
		rows[i] = []any{id}
	}
	return rows
}

// testStore returns a store on a fake connection with a fixed clock.
func testStore(t *testing.T) (*Store, *fakeConn, time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	f := newFakeConn()
	return newStore(f, func() time.Time { return now }), f, now
}
