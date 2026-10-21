// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	ext "github.com/nightCode42/plux3/backend/ext/analytics"
	"github.com/nightCode42/plux3/ee/backend/internal/stdwebhook"
)

// FileExporter is the enterprise implementation of the core's export seam
// (ext.Exporter): the core schedules closed hours per organisation and hands
// each to Export with every connection and size it may use (ANL-011). It
// holds no state.
type FileExporter struct{}

var _ ext.Exporter = FileExporter{}

// Validate says why the settings are not acceptable: an export the licence
// does not list, a granularity that does not exist, or a signing secret that
// is not "whsec_" and base64. It opens no connection.
func (FileExporter) Validate(ctx context.Context, s ext.ExportSettings, secret string) error {
	if s.Parquet.Enabled {
		if err := requireFeature(ctx, edition.FeatureParquetExport); err != nil {
			return err
		}
		if !s.Parquet.Granularity.Valid() {
			return fmt.Errorf("analytics: %q is not a granularity", s.Parquet.Granularity)
		}
	}
	if s.Webhook.Enabled {
		if err := requireFeature(ctx, edition.FeatureWebhookExport); err != nil {
			return err
		}
	}
	if secret != "" {
		if _, err := stdwebhook.ParseSecret(secret); err != nil {
			return fmt.Errorf("analytics: the webhook secret: %w", err)
		}
	}
	return nil
}

// Export writes the window of r to every destination its settings enable and
// returns nil only when all of them accepted it. Files go under keys that
// name the window and messages carry identifiers derived from their events,
// so a window exported again changes nothing at the destination.
func (FileExporter) Export(ctx context.Context, r ext.ExportRequest) error {
	if r.Settings.Parquet.Enabled {
		if err := exportParquet(ctx, r); err != nil {
			return err
		}
	}
	if r.Settings.Webhook.Enabled {
		return exportWebhook(ctx, r)
	}
	return nil
}

// exportParquet writes the kinds of file the settings choose.
func exportParquet(ctx context.Context, r ext.ExportRequest) error {
	if r.Settings.Parquet.Events && r.Events == nil {
		return errors.New("analytics: export events: the store keeps no raw events to read")
	}
	if r.Objects == nil {
		return errors.New("analytics: export Parquet: no object store")
	}
	p, err := NewParquetExporter(ParquetConfig{Events: readerSource{r.Events}, Rollups: r.Rollups, Objects: r.Objects, RowsPerFile: r.RowsPerFile})
	if err != nil {
		return err
	}
	if r.Settings.Parquet.Events {
		if _, err := p.ExportEvents(ctx, r.OrganizationID, r.From, r.To); err != nil {
			return err
		}
	}
	if r.Settings.Parquet.Rollups {
		if _, err := p.ExportRollups(ctx, r.OrganizationID, r.Settings.Parquet.Granularity, r.From, r.To); err != nil {
			return err
		}
	}
	return nil
}

// exportWebhook posts the events of the window in order, one batch at a
// time, and stops at the first batch the receiver does not accept.
func exportWebhook(ctx context.Context, r ext.ExportRequest) error {
	if r.Events == nil {
		return errors.New("analytics: export webhook: the store keeps no raw events to read")
	}
	w, err := NewWebhookExporter(WebhookConfig{Client: r.HTTP, URL: r.Settings.Webhook.URL, Secret: r.Secret, BatchSize: r.BatchSize, Now: r.Now})
	if err != nil {
		return err
	}
	after := ""
	for {
		page, next, err := r.Events.ReadEvents(ctx, r.OrganizationID, r.From, r.To, after, r.BatchSize)
		if err != nil {
			return fmt.Errorf("analytics: export webhook: %w", err)
		}
		if err := w.Export(ctx, r.OrganizationID, recordsOf(page)); err != nil {
			return err
		}
		if len(page) < r.BatchSize {
			return nil
		}
		after = next
	}
}

func recordsOf(events []ext.StoredEvent) []Record {
	out := make([]Record, len(events))
	for i, e := range events {
		out[i] = Record(e)
	}
	return out
}

// readerSource lets the Parquet exporter read events through the core's
// opaque-cursor reader: the opaque cursor travels in Cursor.ID, which the
// exporter only hands back.
type readerSource struct{ r ext.EventReader }

var _ EventSource = readerSource{}

// Events reads one page; it fails when the store keeps no raw events.
func (s readerSource) Events(ctx context.Context, org string, from, to time.Time, after Cursor, limit int) ([]Record, Cursor, error) {
	if s.r == nil {
		return nil, after, errors.New("analytics: no event source")
	}
	page, next, err := s.r.ReadEvents(ctx, org, from, to, after.ID, limit)
	if err != nil {
		return nil, after, fmt.Errorf("analytics: read events: %w", err)
	}
	return recordsOf(page), Cursor{ID: next}, nil
}

var _ ext.EventReader = (*Store)(nil)

// ReadEvents is Events with the cursor as one opaque string: the time of the
// last event and its identifier. It lets the store serve the core's exports.
func (s *Store) ReadEvents(ctx context.Context, org string, from, to time.Time, after string, limit int) ([]ext.StoredEvent, string, error) {
	var cursor Cursor
	if after != "" {
		at, id, ok := strings.Cut(after, "|")
		t, err := time.Parse(time.RFC3339Nano, at)
		if !ok || err != nil {
			return nil, after, errors.New("analytics: read events: the cursor is malformed")
		}
		cursor = Cursor{Time: t, ID: id}
	}
	page, next, err := s.Events(ctx, org, from, to, cursor, limit)
	if err != nil {
		return nil, after, err
	}
	out := make([]ext.StoredEvent, len(page))
	for i, e := range page {
		out[i] = ext.StoredEvent(e)
	}
	if len(page) == 0 {
		return out, after, nil
	}
	return out, next.Time.UTC().Format(time.RFC3339Nano) + "|" + next.ID, nil
}
