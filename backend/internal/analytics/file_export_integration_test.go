// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics_test

import (
	"context"
	"encoding/base64"
	"errors"
	"sync"
	"testing"
	"time"

	ext "github.com/nightCode42/plux3/backend/ext/analytics"
	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/analytics/analyticstest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// testSecret is a signing secret made up for the tests: "whsec_" and base64.
var testSecret = "whsec_" + base64.StdEncoding.EncodeToString([]byte("a test secret of thirty-two byte"))

// recordingExporter is an Exporter that records the windows it is given and
// fails the ones the test names.
type recordingExporter struct {
	mu      sync.Mutex
	windows []ext.ExportRequest
	failAt  time.Time
}

func (*recordingExporter) Validate(context.Context, ext.ExportSettings, string) error { return nil }

func (e *recordingExporter) Export(_ context.Context, r ext.ExportRequest) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r.From.Equal(e.failAt) {
		return errors.New("the receiver is down")
	}
	e.windows = append(e.windows, r)
	return nil
}

func (e *recordingExporter) from() []time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]time.Time, len(e.windows))
	for i, w := range e.windows {
		out[i] = w.From
	}
	return out
}

var parquetOnly = analytics.ExportSettings{Parquet: ext.ParquetSettings{Enabled: true, Events: true, Rollups: true, Granularity: ext.Hour}}

// Verifies: ANL-011, GOV-030.
// Without an enterprise exporter the settings are refused with PLX-8600, and
// reading them shows every export off.
func TestFileExport_RefusedWithoutExporter_ANL_011(t *testing.T) {
	t.Parallel()
	f := analyticstest.New(t, analyticstest.Options{})
	if _, err := f.Analytics.SetFileExport(context.Background(), f.Owner, parquetOnly, ""); code(err) != plxerr.EnterpriseFeatureNotLicensed {
		t.Errorf("SetFileExport = %v, want PLX-8600", err)
	}
	got, err := f.Analytics.GetFileExport(context.Background(), f.Owner)
	if err != nil || got.Settings.Parquet.Enabled || got.Settings.Webhook.Enabled {
		t.Errorf("GetFileExport = %+v, %v; want every export off", got, err)
	}
	if err := f.Analytics.ExportFiles(context.Background()); err != nil {
		t.Errorf("ExportFiles without an exporter = %v", err)
	}
}

// Verifies: ANL-011, SEC-092.
// The settings are checked before they are stored, the signing secret is
// sealed and only its end is ever returned, and an unprivileged principal
// cannot change them.
func TestFileExport_SettingsAndSecret_ANL_011(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{Exporter: &recordingExporter{}})
	bad := map[string]analytics.ExportSettings{
		"granularity": {Parquet: ext.ParquetSettings{Enabled: true, Events: true, Granularity: "7d"}},
		"no kind":     {Parquet: ext.ParquetSettings{Enabled: true}},
		"http":        {Webhook: ext.WebhookSettings{Enabled: true, URL: "http://example.com/hook"}},
		"credentials": {Webhook: ext.WebhookSettings{Enabled: true, URL: "https://user:pw@example.com/hook"}},
	}
	for name, s := range bad {
		if _, err := f.Analytics.SetFileExport(ctx, f.Owner, s, testSecret); err == nil {
			t.Errorf("%s: settings were accepted", name)
		}
	}
	hook := analytics.ExportSettings{Webhook: ext.WebhookSettings{Enabled: true, URL: "https://example.com/hook"}}
	if _, err := f.Analytics.SetFileExport(ctx, f.Owner, hook, ""); err == nil {
		t.Error("a webhook without a signing secret was accepted")
	}
	got, err := f.Analytics.SetFileExport(ctx, f.Owner, hook, testSecret)
	if err != nil || got.SecretHint == "" || got.SecretHint == testSecret || len(got.SecretHint) >= len(testSecret) {
		t.Fatalf("SetFileExport = %+v, %v; want a hint that is not the secret", got, err)
	}
	kept, err := f.Analytics.SetFileExport(ctx, f.Owner, hook, "")
	if err != nil || kept.SecretHint != got.SecretHint {
		t.Errorf("an empty secret changed the stored one: %+v, %v", kept, err)
	}
	if _, err := f.Analytics.SetFileExport(ctx, f.Viewer(), hook, ""); err == nil {
		t.Error("a viewer changed the export settings")
	}
}

// Verifies: ANL-011.
// The export starts with the first hour after the settings are set, delivers
// each closed hour once and in order, and records how far it has come.
func TestFileExport_DeliversClosedHoursOnce_ANL_011(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rec := &recordingExporter{}
	f := analyticstest.New(t, analyticstest.Options{Exporter: rec})
	if _, err := f.Analytics.SetFileExport(ctx, f.Owner, parquetOnly, ""); err != nil {
		t.Fatal(err)
	}
	start := f.Clock.Now().Truncate(time.Hour).Add(time.Hour)
	if err := f.Analytics.ExportFiles(ctx); err != nil || len(rec.from()) != 0 {
		t.Fatalf("an export before the first hour closed: %v, %v", rec.from(), err)
	}
	f.Clock.Advance(3*time.Hour + 10*time.Minute)
	if err := f.Analytics.ExportFiles(ctx); err != nil {
		t.Fatal(err)
	}
	want := []time.Time{start, start.Add(time.Hour)}
	if got := rec.from(); len(got) != 2 || !got[0].Equal(want[0]) || !got[1].Equal(want[1]) {
		t.Fatalf("windows = %v, want %v", got, want)
	}
	if err := f.Analytics.ExportFiles(ctx); err != nil || len(rec.from()) != 2 {
		t.Errorf("a second run exported %d more windows: %v", len(rec.from())-2, err)
	}
	if got, _ := f.Analytics.GetFileExport(ctx, f.Owner); !got.ExportedUntil.Equal(start.Add(2*time.Hour)) || got.LastError != "" {
		t.Errorf("state = %+v, want exported until %v", got, start.Add(2*time.Hour))
	}
}

// Verifies: ANL-011.
// A window that fails is recorded and repeated from its start at the next
// round, after the windows before it were kept as delivered.
func TestFileExport_RepeatsAFailedWindow_ANL_011(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rec := &recordingExporter{}
	f := analyticstest.New(t, analyticstest.Options{Exporter: rec})
	if _, err := f.Analytics.SetFileExport(ctx, f.Owner, parquetOnly, ""); err != nil {
		t.Fatal(err)
	}
	start := f.Clock.Now().Truncate(time.Hour).Add(time.Hour)
	rec.failAt = start.Add(time.Hour)
	f.Clock.Advance(4 * time.Hour)
	if err := f.Analytics.ExportFiles(ctx); err == nil {
		t.Fatal("a failing window did not fail the round")
	}
	got, _ := f.Analytics.GetFileExport(ctx, f.Owner)
	if !got.ExportedUntil.Equal(start.Add(time.Hour)) || got.LastError == "" {
		t.Fatalf("state = %+v, want exported until %v with an error", got, start.Add(time.Hour))
	}
	rec.mu.Lock()
	rec.failAt = time.Time{}
	rec.mu.Unlock()
	if err := f.Analytics.ExportFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if got := rec.from(); len(got) != 2 || !got[1].Equal(start.Add(time.Hour)) {
		t.Errorf("windows = %v, want the failed hour delivered next", got)
	}
	if got, _ := f.Analytics.GetFileExport(ctx, f.Owner); got.LastError != "" {
		t.Errorf("the error stayed after a success: %q", got.LastError)
	}
}
