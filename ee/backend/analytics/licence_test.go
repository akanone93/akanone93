// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/edition"
	ext "github.com/nightCode42/plux3/backend/ext/analytics"
)

// fixedLicence is an edition provider with a fixed status.
type fixedLicence struct{ status edition.Status }

// Status returns the fixed status.
func (f fixedLicence) Status() edition.Status { return f.status }

// licensed returns a context whose licence is in a state and lists features.
func licensed(state edition.State, features ...edition.Feature) context.Context {
	return edition.WithProvider(context.Background(), fixedLicence{status: edition.Status{State: state, Features: features}})
}

// Verifies: ANL-010, GOV-030.
// Without a URL, or without a licence that lists ClickHouse for writing, the
// opener returns no store and no error, so the core keeps PostgreSQL; the
// unlicensed case is a warning that does not quote the URL.
func TestOpener_UnlicensedKeepsPostgreSQL_ANL_010(t *testing.T) {
	t.Parallel()
	const url = "clickhouse://user:s3cret@127.0.0.1:1/plux"
	cases := map[string]struct {
		ctx  context.Context
		url  string
		warn bool
	}{
		"no URL":             {licensed(edition.StateValid, edition.FeatureClickHouse), "", false},
		"no licence":         {context.Background(), url, true},
		"feature not listed": {licensed(edition.StateValid, edition.FeatureParquetExport), url, true},
		"expired":            {licensed(edition.StateExpired, edition.FeatureClickHouse), url, true},
		"invalid":            {licensed(edition.StateInvalid, edition.FeatureClickHouse), url, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var logged bytes.Buffer
			log := slog.New(slog.NewTextHandler(&logged, nil))
			store, err := Opener{URL: c.url}.Open(c.ctx, ext.Settings{Log: log})
			if store != nil || err != nil {
				t.Fatalf("Open = %v, %v; want no store and no error", store, err)
			}
			if got := logged.Len() > 0; got != c.warn {
				t.Errorf("warning logged = %v, want %v: %s", got, c.warn, logged.String())
			}
			if strings.Contains(logged.String(), "s3cret") {
				t.Error("the log holds the password")
			}
		})
	}
}

// Verifies: ANL-010.
// With a licence the opener goes on to the server: a URL that is not a
// ClickHouse DSN, or a server that is not there, is an error that does not
// quote the URL.
func TestOpener_Licensed_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := licensed(edition.StateValid, edition.FeatureClickHouse)
	log := slog.New(slog.DiscardHandler)
	for _, url := range []string{"://s3cret", "clickhouse://user:s3cret@127.0.0.1:1/plux?dial_timeout=1s"} {
		store, err := Opener{URL: url}.Open(ctx, ext.Settings{Log: log})
		if store != nil || err == nil {
			t.Fatalf("Open(%q) = %v, %v; want an error", url, store, err)
		}
		if strings.Contains(err.Error(), "s3cret") {
			t.Errorf("the error quotes the password: %v", err)
		}
	}
}

// Verifies: ANL-010.
// The opener reads the URL from PLUX_ANALYTICS_CLICKHOUSE_URL.
func TestOpenerFromEnv_ANL_010(t *testing.T) {
	t.Parallel()
	o := OpenerFromEnv(func(k string) string {
		if k != "PLUX_ANALYTICS_CLICKHOUSE_URL" {
			t.Errorf("read %q", k)
		}
		return "clickhouse://h"
	})
	if o.URL != "clickhouse://h" {
		t.Errorf("URL = %q", o.URL)
	}
}
