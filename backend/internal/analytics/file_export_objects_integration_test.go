// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics_test

import (
	"context"
	"strings"
	"testing"
	"time"

	ext "github.com/nightCode42/plux3/backend/ext/analytics"
	"github.com/nightCode42/plux3/backend/internal/analytics/analyticstest"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// puttingExporter is an Exporter that stores one file per window through
// the object store the core hands it, and reads events through the reader.
type puttingExporter struct {
	recordingExporter
	events []ext.StoredEvent
}

func (e *puttingExporter) Export(ctx context.Context, r ext.ExportRequest) error {
	key := "analytics/org=" + r.OrganizationID + "/events/date=2026-10-19/window-0001.parquet"
	if err := r.Objects.Put(ctx, key, strings.NewReader("rows"), "application/vnd.apache.parquet"); err != nil {
		return err //nolint:wrapcheck // a test double
	}
	var after string
	for {
		page, next, err := r.Events.ReadEvents(ctx, r.OrganizationID, r.From, r.To, after, 2)
		if err != nil {
			return err //nolint:wrapcheck // a test double
		}
		e.events = append(e.events, page...)
		if len(page) < 2 {
			return nil
		}
		after = next
	}
}

// Verifies: ANL-011.
// The core hands an exporter an object store that keeps its file under the
// "exports" prefix and an event reader that returns each event of the window
// once, however the pages fall.
func TestFileExport_ObjectsAndEvents_ANL_011(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := objects.NewFilesystem(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	exp := &puttingExporter{}
	f := analyticstest.New(t, analyticstest.Options{Exporter: exp, Objects: store})
	if _, err := f.Analytics.SetFileExport(ctx, f.Owner, parquetOnly, ""); err != nil {
		t.Fatal(err)
	}
	start := f.Clock.Now().Truncate(time.Hour).Add(time.Hour)
	var samples []sample
	for i := range 5 {
		samples = append(samples, sample{at: start.Add(time.Duration(i) * time.Minute), release: 1, page: "/a", value: 1})
	}
	write(t, f, samples)
	f.Clock.Advance(2*time.Hour + 10*time.Minute)
	if err := f.Analytics.ExportFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if len(exp.events) != 5 {
		t.Errorf("read %d events over pages of 2, want 5", len(exp.events))
	}
	key := "exports/analytics/org=" + f.Org + "/events/date=2026-10-19/window-0001.parquet"
	if body, _, err := store.Get(ctx, key); err != nil || string(body) != "rows" {
		t.Errorf("Get(%s) = %q, %v", key, body, err)
	}
}
