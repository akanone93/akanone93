// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/nightCode42/plux3/backend/edition"
	ext "github.com/nightCode42/plux3/backend/ext/analytics"
)

// parquetContentType is the media type the files are stored with.
const parquetContentType = "application/vnd.apache.parquet"

// EventRow is one row of an events file. Times are UTC, in microseconds.
// Fields is the event's JSON object as text. Identifiers are canonical
// UUIDs as text. The file is Snappy-compressed.
type EventRow struct {
	ID              string    `parquet:"id"`
	OrganizationID  string    `parquet:"organization_id"`
	AppID           string    `parquet:"app_id"`
	EnvironmentID   string    `parquet:"environment_id"`
	DeviceID        string    `parquet:"device_id"`
	Name            string    `parquet:"name"`
	Time            time.Time `parquet:"time,timestamp(microsecond)"`
	ReleaseSequence int64     `parquet:"release_sequence"`
	PluginKey       string    `parquet:"plugin_key"`
	Route           string    `parquet:"route"`
	Category        string    `parquet:"category"`
	DeviceClass     string    `parquet:"device_class"`
	Locale          string    `parquet:"locale"`
	Fields          string    `parquet:"fields"`
}

// RollupRow is one row of a rollups file: a bucket of one series of one app.
// Dimensions that are not set are empty strings and Release is 0. Sum, Min,
// Max and the percentiles are durations in milliseconds; Histogram holds the
// counts of the sixteen fixed buckets the percentiles come from.
type RollupRow struct {
	AppID         string    `parquet:"app_id"`
	EventName     string    `parquet:"event_name"`
	Granularity   string    `parquet:"granularity"`
	Bucket        time.Time `parquet:"bucket,timestamp(microsecond)"`
	EnvironmentID string    `parquet:"environment_id"`
	Release       int64     `parquet:"release_sequence"`
	Plugin        string    `parquet:"plugin_key"`
	Page          string    `parquet:"page"`
	DeviceClass   string    `parquet:"device_class"`
	Locale        string    `parquet:"locale"`
	Events        int64     `parquet:"events"`
	ValueCount    int64     `parquet:"value_count"`
	Sum           float64   `parquet:"value_sum"`
	Min           float64   `parquet:"value_min"`
	Max           float64   `parquet:"value_max"`
	P50           float64   `parquet:"p50"`
	P90           float64   `parquet:"p90"`
	P99           float64   `parquet:"p99"`
	Histogram     []int64   `parquet:"histogram,list"`
}

// ObjectStore is where files go. The core passes in its object storage;
// Put stores the body under the key, replacing an object of that name, so
// that an export repeated after a failure writes the same files again.
type ObjectStore interface {
	Put(ctx context.Context, key string, body io.Reader, contentType string) error
}

// EventSource reads raw events. Store implements it.
type EventSource interface {
	Events(ctx context.Context, org string, from, to time.Time, after Cursor, limit int) ([]Record, Cursor, error)
}

// RollupSource reads rollups. Any analytics store implements it.
type RollupSource interface {
	Window(ctx context.Context, org string, g ext.Granularity, from, to time.Time) ([]ext.Point, error)
}

// ParquetConfig configures a ParquetExporter.
type ParquetConfig struct {
	Events  EventSource
	Rollups RollupSource
	Objects ObjectStore
	// RowsPerFile bounds the rows of a file, and so the memory of an
	// export; it comes from the installation's configuration.
	RowsPerFile int
}

// ParquetExporter writes events and rollups of an organisation to object
// storage as Parquet files (ANL-011). Keys are
// analytics/org=<organisation>/<kind>/date=<UTC day of the window start>/
// <window start>-<window end>-<part>.parquet, with the granularity as a
// further level for rollups, so a window exported twice replaces its files.
type ParquetExporter struct {
	c ParquetConfig
}

// NewParquetExporter validates a configuration and returns the exporter.
func NewParquetExporter(c ParquetConfig) (*ParquetExporter, error) {
	if c.Objects == nil || c.RowsPerFile <= 0 {
		return nil, errors.New("analytics: a Parquet exporter needs an object store and a positive file size")
	}
	return &ParquetExporter{c: c}, nil
}

// ExportEvents writes the events of [from, to) and returns the keys of the
// files, none when the window holds no event. It refuses the write without a
// licence that lists edition.FeatureParquetExport.
func (p *ParquetExporter) ExportEvents(ctx context.Context, org string, from, to time.Time) ([]string, error) {
	if err := requireFeature(ctx, edition.FeatureParquetExport); err != nil {
		return nil, err
	}
	if p.c.Events == nil {
		return nil, errors.New("analytics: export events: no event source")
	}
	org, err := canonicalUUID(org)
	if err != nil {
		return nil, fmt.Errorf("analytics: export events: the organisation: %w", err)
	}
	var keys []string
	var cursor Cursor
	for part := 1; ; part++ {
		page, next, err := p.c.Events.Events(ctx, org, from, to, cursor, p.c.RowsPerFile)
		if err != nil {
			return keys, fmt.Errorf("analytics: export events: %w", err)
		}
		if len(page) == 0 {
			return keys, nil
		}
		key := objectKey(org, "events", "", from, to, part)
		if err := putFile(ctx, p.c.Objects, key, eventRows(page)); err != nil {
			return keys, err
		}
		keys = append(keys, key)
		if len(page) < p.c.RowsPerFile {
			return keys, nil
		}
		cursor = next
	}
}

// ExportRollups writes the rollups of one granularity in [from, to) and
// returns the keys of the files, none when the window holds no rollup. It
// refuses the write without a licence that lists edition.FeatureParquetExport.
func (p *ParquetExporter) ExportRollups(ctx context.Context, org string, g ext.Granularity, from, to time.Time) ([]string, error) {
	if err := requireFeature(ctx, edition.FeatureParquetExport); err != nil {
		return nil, err
	}
	if p.c.Rollups == nil {
		return nil, errors.New("analytics: export rollups: no rollup source")
	}
	org, err := canonicalUUID(org)
	if err != nil {
		return nil, fmt.Errorf("analytics: export rollups: the organisation: %w", err)
	}
	points, err := p.c.Rollups.Window(ctx, org, g, from, to)
	if err != nil {
		return nil, fmt.Errorf("analytics: export rollups: %w", err)
	}
	var keys []string
	part := 0
	for chunk := range slices.Chunk(points, p.c.RowsPerFile) {
		part++
		key := objectKey(org, "rollups", string(g), from, to, part)
		if err := putFile(ctx, p.c.Objects, key, rollupRows(chunk)); err != nil {
			return keys, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// putFile encodes rows and stores them under a key.
func putFile[T any](ctx context.Context, objects ObjectStore, key string, rows []T) error {
	data, err := encode(rows)
	if err != nil {
		return err
	}
	if err := objects.Put(ctx, key, bytes.NewReader(data), parquetContentType); err != nil {
		return fmt.Errorf("analytics: store %s: %w", key, err)
	}
	return nil
}

// encode writes rows as one Snappy-compressed Parquet file.
func encode[T any](rows []T) ([]byte, error) {
	var buf bytes.Buffer
	w := parquet.NewGenericWriter[T](&buf, parquet.Compression(&parquet.Snappy))
	if _, err := w.Write(rows); err != nil {
		return nil, fmt.Errorf("analytics: encode Parquet: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("analytics: finish Parquet: %w", err)
	}
	return buf.Bytes(), nil
}

// objectKey names a file. The window is in UTC seconds, so the key does not
// change when the same window is exported again.
func objectKey(org, kind, granularity string, from, to time.Time, part int) string {
	key := "analytics/org=" + org + "/" + kind + "/"
	if granularity != "" {
		key += "granularity=" + granularity + "/"
	}
	return fmt.Sprintf("%sdate=%s/%d-%d-%04d.parquet", key, from.UTC().Format("2006-01-02"), from.Unix(), to.Unix(), part)
}

// eventRows converts events to file rows.
func eventRows(records []Record) []EventRow {
	rows := make([]EventRow, len(records))
	for i, r := range records {
		rows[i] = EventRow{
			ID: r.ID, OrganizationID: r.OrganizationID, AppID: r.AppID, EnvironmentID: r.EnvironmentID, DeviceID: r.DeviceID,
			Name: r.Name, Time: r.Time.UTC(), ReleaseSequence: r.ReleaseSequence, PluginKey: r.PluginKey, Route: r.Route,
			Category: r.Category, DeviceClass: r.DeviceClass, Locale: r.Locale, Fields: string(r.Fields),
		}
	}
	return rows
}

// rollupRows converts points to file rows.
func rollupRows(points []ext.Point) []RollupRow {
	rows := make([]RollupRow, len(points))
	for i, p := range points {
		rows[i] = RollupRow{
			AppID: p.AppID, EventName: p.EventName, Granularity: string(p.Granularity), Bucket: p.Bucket.UTC(),
			EnvironmentID: p.Group.EnvironmentID, Release: p.Group.Release, Plugin: p.Group.Plugin, Page: p.Group.Page,
			DeviceClass: p.Group.DeviceClass, Locale: p.Group.Locale, Events: p.Events, ValueCount: p.ValueCount,
			Sum: p.Sum, Min: p.Min, Max: p.Max, P50: p.P50, P90: p.P90, P99: p.P99, Histogram: slices.Clone(p.Histogram[:]),
		}
	}
	return rows
}
