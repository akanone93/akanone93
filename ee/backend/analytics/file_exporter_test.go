// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	ext "github.com/nightCode42/plux3/backend/ext/analytics"
)

// opaqueEvents serves a fixed list of events through the core's opaque
// cursor, which here is the identifier of the last event.
type opaqueEvents struct{ records []Record }

func (o opaqueEvents) ReadEvents(_ context.Context, _ string, _, _ time.Time, after string, limit int) ([]ext.StoredEvent, string, error) {
	start := 0
	if after != "" {
		start = slices.IndexFunc(o.records, func(r Record) bool { return r.ID == after }) + 1
	}
	end := min(start+limit, len(o.records))
	out := make([]ext.StoredEvent, 0, end-start)
	for _, r := range o.records[start:end] {
		out = append(out, ext.StoredEvent(r))
	}
	if len(out) == 0 {
		return nil, after, nil
	}
	return out, out[len(out)-1].ID, nil
}

func exportRequest(t *testing.T, s ext.ExportSettings, objects ObjectStore, r *receiver) ext.ExportRequest {
	t.Helper()
	req := ext.ExportRequest{
		OrganizationID: testOrg, From: exportFrom, To: exportTo, Settings: s,
		Events: opaqueEvents{records: sampleRecords(5)}, Rollups: fixedWindow{points: []ext.Point{{}}}, Objects: objects,
		RowsPerFile: 2, BatchSize: 2, Now: func() time.Time { return exportTo },
	}
	if r != nil {
		req.HTTP, req.Secret = r.srv.Client(), vectorSecret(t)
		req.Settings.Webhook.URL = r.srv.URL
	}
	return req
}

var allExports = ext.ExportSettings{
	Parquet: ext.ParquetSettings{Enabled: true, Events: true, Rollups: true, Granularity: ext.Hour},
	Webhook: ext.WebhookSettings{Enabled: true},
}

// Verifies: ANL-011.
// A window goes to every enabled destination: Parquet files of events and
// rollups under the window's keys, and every event in signed batches.
func TestFileExporter_ExportsEveryDestination_ANL_011(t *testing.T) {
	t.Parallel()
	ctx := licensed(edition.StateValid, edition.FeatureParquetExport, edition.FeatureWebhookExport)
	objects, r := newMemoryObjects(), newReceiver(t, http.StatusNoContent)
	if err := (FileExporter{}).Export(ctx, exportRequest(t, allExports, objects, r)); err != nil {
		t.Fatal(err)
	}
	var events, rollups int
	for _, k := range objects.keys() {
		events += strings.Count(k, "/events/")
		rollups += strings.Count(k, "/rollups/")
	}
	if events != 3 || rollups != 1 {
		t.Errorf("keys %v: %d event files and %d rollup files, want 3 and 1", objects.keys(), events, rollups)
	}
	total := 0
	for _, m := range r.messages {
		var env webhookEnvelope
		if err := json.Unmarshal(m.body, &env); err != nil {
			t.Fatal(err)
		}
		total += len(env.Data.Events)
	}
	if len(r.messages) != 3 || total != 5 {
		t.Errorf("%d messages with %d events, want 3 with 5", len(r.messages), total)
	}
}

// Verifies: ANL-011, GOV-030.
// Without the licence for an export, validating its settings and running it
// are refused with PLX-8600 and nothing is written or sent.
func TestFileExporter_RefusedWithoutLicence_ANL_011(t *testing.T) {
	t.Parallel()
	ctx := licensed(edition.StateExpired)
	objects, r := newMemoryObjects(), newReceiver(t, http.StatusNoContent)
	if err := (FileExporter{}).Validate(ctx, allExports, ""); err == nil || !strings.Contains(err.Error(), "PLX-8600") {
		t.Errorf("Validate = %v, want PLX-8600", err)
	}
	if err := (FileExporter{}).Export(ctx, exportRequest(t, allExports, objects, r)); err == nil || !strings.Contains(err.Error(), "PLX-8600") {
		t.Errorf("Export = %v, want PLX-8600", err)
	}
	if len(objects.keys()) != 0 || len(r.messages) != 0 {
		t.Errorf("%d files and %d messages written without a licence", len(objects.keys()), len(r.messages))
	}
}

// Verifies: ANL-011.
// A failing destination fails the window, so the core repeats it: a
// receiver that refuses stops the export at the first batch, and a store
// that keeps no raw events fails an event export instead of writing nothing.
func TestFileExporter_Failures_ANL_011(t *testing.T) {
	t.Parallel()
	ctx := licensed(edition.StateValid, edition.FeatureParquetExport, edition.FeatureWebhookExport)
	r := newReceiver(t, http.StatusInternalServerError)
	if err := (FileExporter{}).Export(ctx, exportRequest(t, ext.ExportSettings{Webhook: allExports.Webhook}, nil, r)); err == nil {
		t.Error("a receiver that refuses did not fail the export")
	}
	if len(r.messages) != 1 {
		t.Errorf("%d messages after a refusal, want the export to stop at the first", len(r.messages))
	}
	noEvents := exportRequest(t, allExports, newMemoryObjects(), nil)
	noEvents.Events = nil
	if err := (FileExporter{}).Export(ctx, noEvents); err == nil {
		t.Error("an event export without an event reader succeeded")
	}
	noObjects := exportRequest(t, allExports, nil, nil)
	if err := (FileExporter{}).Export(ctx, noObjects); err == nil {
		t.Error("a Parquet export without an object store succeeded")
	}
}

// Verifies: ANL-011, SEC-092.
// A signing secret that is not "whsec_" and base64 is refused when the
// settings are set, not when the first window fails.
func TestFileExporter_ValidateSecret_ANL_011(t *testing.T) {
	t.Parallel()
	ctx := licensed(edition.StateValid, edition.FeatureParquetExport, edition.FeatureWebhookExport)
	if err := (FileExporter{}).Validate(ctx, allExports, "not-a-secret"); err == nil {
		t.Error("a malformed signing secret was accepted")
	}
	if err := (FileExporter{}).Validate(ctx, allExports, vectorSecret(t)); err != nil {
		t.Errorf("Validate(valid) = %v", err)
	}
	bad := allExports
	bad.Parquet.Granularity = "7d"
	if err := (FileExporter{}).Validate(ctx, bad, ""); err == nil {
		t.Error("an unknown granularity was accepted")
	}
}
