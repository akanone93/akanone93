// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/format"

	"github.com/nightCode42/plux3/backend/edition"
	ext "github.com/nightCode42/plux3/backend/ext/analytics"
)

// memoryObjects is an object store in memory.
type memoryObjects struct {
	mu    sync.Mutex
	files map[string][]byte
	types map[string]string
	fail  error
}

func newMemoryObjects() *memoryObjects {
	return &memoryObjects{files: map[string][]byte{}, types: map[string]string{}}
}

// Put stores the body.
func (m *memoryObjects) Put(_ context.Context, key string, body io.Reader, contentType string) error {
	if m.fail != nil {
		return m.fail
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[key], m.types[key] = data, contentType
	return nil
}

func (m *memoryObjects) keys() []string {
	var out []string
	for k := range m.files {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// pagedEvents serves a fixed list of events by cursor.
type pagedEvents struct{ records []Record }

func (p pagedEvents) Events(_ context.Context, _ string, _, _ time.Time, after Cursor, limit int) ([]Record, Cursor, error) {
	start := 0
	if after.ID != "" {
		start = slices.IndexFunc(p.records, func(r Record) bool { return r.ID == after.ID }) + 1
	}
	end := min(start+limit, len(p.records))
	page := p.records[start:end]
	if len(page) == 0 {
		return nil, after, nil
	}
	return page, Cursor{Time: page[len(page)-1].Time, ID: page[len(page)-1].ID}, nil
}

// fixedWindow serves fixed points.
type fixedWindow struct{ points []ext.Point }

func (f fixedWindow) Window(context.Context, string, ext.Granularity, time.Time, time.Time) ([]ext.Point, error) {
	return f.points, nil
}

var (
	exportFrom = time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	exportTo   = exportFrom.Add(time.Hour)
)

func parquetExporter(t *testing.T, objects ObjectStore, rows int, points []ext.Point) *ParquetExporter {
	t.Helper()
	p, err := NewParquetExporter(ParquetConfig{
		Events: pagedEvents{records: sampleRecords(5)}, Rollups: fixedWindow{points: points}, Objects: objects, RowsPerFile: rows,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// openFile parses a stored file and checks that it is Snappy-compressed.
func openFile(t *testing.T, data []byte) *parquet.File {
	t.Helper()
	f, err := parquet.OpenFile(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, rg := range f.Metadata().RowGroups {
		for _, c := range rg.Columns {
			if c.MetaData.Codec != format.Snappy {
				t.Errorf("column %v uses codec %v, want Snappy", c.MetaData.PathInSchema, c.MetaData.Codec)
			}
		}
	}
	return f
}

// Verifies: ANL-011.
// Events are written in files of at most RowsPerFile rows, Snappy-compressed,
// and read back with the declared schema and the same values.
func TestParquetExportEvents_ANL_011(t *testing.T) {
	t.Parallel()
	objects := newMemoryObjects()
	ctx := licensed(edition.StateValid, edition.FeatureParquetExport)
	keys, err := parquetExporter(t, objects, 2, nil).ExportEvents(ctx, testOrg, exportFrom, exportTo)
	if err != nil {
		t.Fatal(err)
	}
	if want := objects.keys(); !slices.Equal(keys, want) || len(keys) != 3 {
		t.Fatalf("keys = %v, stored %v; want 3 files", keys, want)
	}
	if !strings.HasPrefix(keys[0], "analytics/org="+testOrg+"/events/date=2026-10-20/") || !strings.HasSuffix(keys[0], "-0001.parquet") {
		t.Errorf("first key = %s", keys[0])
	}
	var got []EventRow
	for _, k := range keys {
		if objects.types[k] != "application/vnd.apache.parquet" {
			t.Errorf("%s has content type %q", k, objects.types[k])
		}
		f := openFile(t, objects.files[k])
		if want := parquet.SchemaOf(new(EventRow)).String(); f.Schema().String() != want {
			t.Errorf("schema = %s, want %s", f.Schema(), want)
		}
		rows, err := parquet.Read[EventRow](bytes.NewReader(objects.files[k]), int64(len(objects.files[k])))
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, rows...)
	}
	if want := eventRows(sampleRecords(5)); !reflect.DeepEqual(got, want) {
		t.Errorf("rows read back:\n%+v\nwant\n%+v", got, want)
	}
}

// Verifies: ANL-011.
// The declared columns of an events file and a rollups file are these names
// and types; a change to either file's schema is a change to the export
// contract and fails here.
func TestParquetSchemas_ANL_011(t *testing.T) {
	t.Parallel()
	columns := func(v any) []string {
		var out []string
		for _, f := range parquet.SchemaOf(v).Fields() {
			out = append(out, f.Name()+":"+f.Type().String())
		}
		return out
	}
	const ts = "TIMESTAMP(isAdjustedToUTC=true,unit=MICROS)"
	events := "id:STRING organization_id:STRING app_id:STRING environment_id:STRING device_id:STRING name:STRING time:" + ts +
		" release_sequence:INT(64,true) plugin_key:STRING route:STRING category:STRING device_class:STRING locale:STRING fields:STRING"
	if got := strings.Join(columns(new(EventRow)), " "); got != events {
		t.Errorf("events columns:\n%s\nwant\n%s", got, events)
	}
	rollups := "app_id:STRING event_name:STRING granularity:STRING bucket:" + ts + " environment_id:STRING release_sequence:INT(64,true) " +
		"plugin_key:STRING page:STRING device_class:STRING locale:STRING events:INT(64,true) value_count:INT(64,true) " +
		"value_sum:DOUBLE value_min:DOUBLE value_max:DOUBLE p50:DOUBLE p90:DOUBLE p99:DOUBLE histogram:LIST"
	if got := strings.Join(columns(new(RollupRow)), " "); got != rollups {
		t.Errorf("rollup columns:\n%s\nwant\n%s", got, rollups)
	}
}

// Verifies: ANL-011.
// Rollups are written with their histogram and read back unchanged.
func TestParquetExportRollups_ANL_011(t *testing.T) {
	t.Parallel()
	var h ext.Histogram
	h[3], h[7] = 4, 2
	points := []ext.Point{
		{
			AppID: testApp, EventName: "screen_view", Granularity: ext.Hour, Bucket: exportFrom, Group: ext.Group{EnvironmentID: testEnv, Release: 3, Page: "/home"},
			Events: 9, ValueCount: 6, Sum: 610, Min: 3, Max: 200, P50: 12, P90: 150, P99: 199, Histogram: h,
		},
		{AppID: testApp, EventName: "session_start", Granularity: ext.Hour, Bucket: exportFrom.Add(time.Hour), Events: 2},
		{AppID: testApp, EventName: "render_perf", Granularity: ext.Hour, Bucket: exportFrom, Events: 1},
	}
	objects := newMemoryObjects()
	ctx := licensed(edition.StateValid, edition.FeatureParquetExport)
	keys, err := parquetExporter(t, objects, 2, points).ExportRollups(ctx, testOrg, ext.Hour, exportFrom, exportTo)
	if err != nil || len(keys) != 2 {
		t.Fatalf("ExportRollups = %v, %v; want 2 files", keys, err)
	}
	if !strings.Contains(keys[0], "/rollups/granularity=1h/date=2026-10-20/") {
		t.Errorf("key = %s", keys[0])
	}
	var got []RollupRow
	for _, k := range keys {
		openFile(t, objects.files[k])
		rows, err := parquet.Read[RollupRow](bytes.NewReader(objects.files[k]), int64(len(objects.files[k])))
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, rows...)
	}
	if want := rollupRows(points); !reflect.DeepEqual(got, want) {
		t.Errorf("rows read back:\n%+v\nwant\n%+v", got, want)
	}
	if len(got[0].Histogram) != ext.Buckets || got[0].Histogram[3] != 4 {
		t.Errorf("histogram = %v", got[0].Histogram)
	}
}

// Verifies: ANL-011.
// An export of the same window names the same files, so a repeat replaces
// them, and a window without events writes none.
func TestParquetExport_KeysAreStable_ANL_011(t *testing.T) {
	t.Parallel()
	ctx := licensed(edition.StateValid, edition.FeatureParquetExport)
	objects := newMemoryObjects()
	p := parquetExporter(t, objects, 10, nil)
	first, err := p.ExportEvents(ctx, testOrg, exportFrom, exportTo)
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.ExportEvents(ctx, testOrg, exportFrom, exportTo)
	if err != nil || !slices.Equal(first, second) || len(objects.files) != 1 {
		t.Errorf("repeat = %v, %v; stored %d files", second, err, len(objects.files))
	}
	empty, err := (&ParquetExporter{c: ParquetConfig{Events: pagedEvents{}, Objects: newMemoryObjects(), RowsPerFile: 3}}).ExportEvents(ctx, testOrg, exportFrom, exportTo)
	if err != nil || len(empty) != 0 {
		t.Errorf("an empty window gave %v, %v", empty, err)
	}
}

// Verifies: ANL-011, GOV-030.
// Without a licence that lists Parquet export for writing, nothing is
// written and the error is PLX-8600.
func TestParquetExport_RefusedWithoutLicence_ANL_011(t *testing.T) {
	t.Parallel()
	objects := newMemoryObjects()
	p := parquetExporter(t, objects, 2, nil)
	for name, ctx := range map[string]context.Context{
		"none":    context.Background(),
		"other":   licensed(edition.StateValid, edition.FeatureWebhookExport),
		"expired": licensed(edition.StateExpired, edition.FeatureParquetExport),
	} {
		if _, err := p.ExportEvents(ctx, testOrg, exportFrom, exportTo); err == nil || !strings.Contains(err.Error(), "PLX-8600") {
			t.Errorf("%s: ExportEvents = %v, want PLX-8600", name, err)
		}
		if _, err := p.ExportRollups(ctx, testOrg, ext.Hour, exportFrom, exportTo); err == nil || !strings.Contains(err.Error(), "PLX-8600") {
			t.Errorf("%s: ExportRollups = %v, want PLX-8600", name, err)
		}
	}
	if len(objects.files) != 0 {
		t.Errorf("%d files were written", len(objects.files))
	}
}

// Verifies: ANL-011.
// An object store that fails stops the export and the error says which file;
// a hostile organisation and a bad configuration are refused.
func TestParquetExport_Failures_ANL_011(t *testing.T) {
	t.Parallel()
	ctx := licensed(edition.StateValid, edition.FeatureParquetExport)
	failing := newMemoryObjects()
	failing.fail = errors.New("disk full")
	if _, err := parquetExporter(t, failing, 2, nil).ExportEvents(ctx, testOrg, exportFrom, exportTo); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("ExportEvents = %v", err)
	}
	if _, err := parquetExporter(t, newMemoryObjects(), 2, nil).ExportEvents(ctx, hostile, exportFrom, exportTo); err == nil {
		t.Error("a hostile organisation was accepted")
	}
	if _, err := NewParquetExporter(ParquetConfig{Objects: newMemoryObjects()}); err == nil {
		t.Error("a zero file size was accepted")
	}
	if _, err := NewParquetExporter(ParquetConfig{RowsPerFile: 1}); err == nil {
		t.Error("a missing object store was accepted")
	}
}
