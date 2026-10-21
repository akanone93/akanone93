// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"github.com/nightCode42/plux3/backend/edition"
	ext "github.com/nightCode42/plux3/backend/ext/analytics"
	"github.com/nightCode42/plux3/backend/ext/analytics/storetest"
)

// liveURLVariable names the ClickHouse server the live tests run against, a
// DSN such as clickhouse://default:@localhost:9000/default. Without it they
// are skipped: CI and a plain `go test` run only the hermetic tests.
const liveURLVariable = "PLUX_TEST_CLICKHOUSE_URL"

// randomUUID returns a random identifier, so that runs against a shared
// server do not meet each other's rows.
func randomUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	b[6], b[8] = b[6]&0x0f|0x70, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// liveStore opens the store on the server named by PLUX_TEST_CLICKHOUSE_URL
// or skips the test.
func liveStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv(liveURLVariable)
	if url == "" {
		t.Skipf("%s is not set: the ClickHouse tests need a server", liveURLVariable)
	}
	opts, err := clickhouse.ParseDSN(url)
	if err != nil {
		t.Fatalf("%s is not a ClickHouse DSN", liveURLVariable)
	}
	db, err := clickhouse.Open(opts)
	if err != nil {
		t.Fatal(err)
	}
	s := newStore(db, nil)
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Prepare(context.Background(), time.Now(), 1); err != nil {
		t.Fatal(err)
	}
	return s
}

// Verifies: ANL-010.
// On a real ClickHouse server the store answers the conformance queries with
// the series the PostgreSQL store returns for the same events, which checks
// the SQL the hermetic tests cannot run. Runs only with
// PLUX_TEST_CLICKHOUSE_URL.
func TestLiveStoreConformance_ANL_010(t *testing.T) {
	s := liveStore(t)
	subject := storetest.Subject{
		Store: s, Organization: randomUUID(t), App: randomUUID(t), Environment: randomUUID(t), Device: randomUUID(t),
	}
	subject.Rollup = func(ctx context.Context) error {
		return s.Rollup(ctx, subject.Organization, time.Now().Add(time.Hour))
	}
	t.Cleanup(func() {
		_, _ = s.Purge(context.Background(), subject.Organization, ext.Purge{
			RawBefore: time.Now().AddDate(10, 0, 0), RollupsBefore: time.Now().AddDate(10, 0, 0),
		})
	})
	storetest.Conformance(t, subject)

	purged, err := s.Purge(context.Background(), subject.Organization, ext.Purge{RawBefore: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if purged.Raw == 0 {
		t.Error("Purge removed no raw events")
	}
	left, _, err := s.Events(context.Background(), subject.Organization, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC), Cursor{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range left {
		if e.Time.Before(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)) {
			t.Errorf("event %s of %s survived the purge", e.ID, e.Time)
		}
	}
}

// Verifies: ANL-010, SEC-161.
// On a real server one organisation's rows are invisible to another.
func TestLiveOrganizationIsolation_ANL_010(t *testing.T) {
	s := liveStore(t)
	ctx := context.Background()
	orgA, orgB := randomUUID(t), randomUUID(t)
	at := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	err := s.WriteEvents(ctx, ext.Batch{
		OrganizationID: orgA, AppID: randomUUID(t), EnvironmentID: randomUUID(t), DeviceID: randomUUID(t),
		Events: []ext.Event{{ID: randomUUID(t), Name: "screen_view", Time: at}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.Purge(ctx, orgA, ext.Purge{RawBefore: time.Now().AddDate(10, 0, 0)})
	})
	seen, _, err := s.Events(ctx, orgB, at.Add(-time.Hour), at.Add(time.Hour), Cursor{}, 10)
	if err != nil || len(seen) != 0 {
		t.Errorf("organisation B read %d events of A (%v)", len(seen), err)
	}
	own, _, err := s.Events(ctx, orgA, at.Add(-time.Hour), at.Add(time.Hour), Cursor{}, 10)
	if err != nil || len(own) != 1 {
		t.Errorf("organisation A read %d events (%v), want its own", len(own), err)
	}
}

// Verifies: ANL-010, GOV-030.
// With a licence that lists ClickHouse, the opener connects to a real server,
// creates the tables and returns a store the server can close.
func TestLiveOpener_ANL_010(t *testing.T) {
	url := os.Getenv(liveURLVariable)
	if url == "" {
		t.Skipf("%s is not set: the ClickHouse tests need a server", liveURLVariable)
	}
	ctx := licensed(edition.StateValid, edition.FeatureClickHouse)
	store, err := Opener{URL: url}.Open(ctx, ext.Settings{Log: slog.New(slog.DiscardHandler)})
	if err != nil || store == nil {
		t.Fatalf("Open = %v, %v", store, err)
	}
	closer, ok := store.(io.Closer)
	if !ok || closer.Close() != nil {
		t.Errorf("the store is not a closer that closes: %T", store)
	}
}

// Verifies: ANL-010.
// On a real server Compact drops the monthly partition that ends before the
// time. The test writes into March 2001, a month no deployment holds data
// for, and drops only that partition.
func TestLiveCompact_ANL_010(t *testing.T) {
	s := liveStore(t)
	ctx := context.Background()
	org := randomUUID(t)
	at := time.Date(2001, 3, 10, 9, 0, 0, 0, time.UTC)
	err := s.WriteEvents(ctx, ext.Batch{
		OrganizationID: org, AppID: randomUUID(t), EnvironmentID: randomUUID(t), DeviceID: randomUUID(t),
		Events: []ext.Event{{ID: randomUUID(t), Name: "screen_view", Time: at}},
	})
	if err != nil {
		t.Fatal(err)
	}
	dropped, err := s.Compact(ctx, time.Date(2001, 4, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if dropped < 1 {
		t.Errorf("Compact dropped %d partitions, want March 2001", dropped)
	}
	left, _, err := s.Events(ctx, org, at.Add(-time.Hour), at.Add(time.Hour), Cursor{}, 10)
	if err != nil || len(left) != 0 {
		t.Errorf("%d events survived the partition drop (%v)", len(left), err)
	}
}
