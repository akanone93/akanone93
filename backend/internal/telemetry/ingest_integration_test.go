// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package telemetry_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/analytics/analyticstest"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/telemetry"
)

// views are n screen views at the fixture's clock.
func views(f *analyticstest.Fixture, n int) []telemetry.Event {
	out := make([]telemetry.Event, n)
	for i := range out {
		out[i] = telemetry.Event{Name: "screen_view", Time: f.Clock.Now(), Route: fmt.Sprintf("/page/%d", i), Fields: []byte(`{"duration_ms": 12}`)}
	}
	return out
}

func tightened(t *testing.T, key limits.Key, v int64) limits.Set {
	t.Helper()
	set, err := limits.Defaults().Tighten(key, limits.ScopeInstallation, v)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// Verifies: ANL-010, LIM-001.
// A device may send telemetry.eventsPerDevicePerHour events in an hour: the
// surplus is dropped and reported as PLX-8300, one device's surplus does not
// count against another, and the next hour starts afresh.
func TestIngestQuota_ANL_010_LIM_001(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{Limits: tightened(t, limits.TelemetryEventsPerDevicePerHour, 5)})
	svc := f.Ingestion(t, telemetry.Options{Counter: cache.NewMemory(f.Clock.Now)})
	src := f.Source(f.Device)

	if n, diags, err := svc.Ingest(ctx, src, views(f, 3)); err != nil || n != 3 || len(diags) != 0 {
		t.Fatalf("under the quota: %d %v %v", n, diags, err)
	}
	n, diags, err := svc.Ingest(ctx, src, views(f, 4))
	if err != nil || n != 2 || len(diags) != 2 {
		t.Fatalf("over the quota: %d %v %v", n, diags, err)
	}
	for i, d := range diags {
		if d.Code != plxerr.IngestionOverQuota || d.Path != fmt.Sprintf("/events/%d", i+2) {
			t.Errorf("diagnostic %d = %s at %s", i, d.Code, d.Path)
		}
	}
	if n, diags, _ := svc.Ingest(ctx, src, views(f, 1)); n != 0 || len(diags) != 1 {
		t.Errorf("a device over its quota sends again: %d accepted, %v", n, diags)
	}
	if got := f.Count(t, "screen_view"); got != 5 {
		t.Errorf("%d events stored, want 5: the quota, no more", got)
	}

	other, _ := f.NewDevice(t)
	if n, _, err := svc.Ingest(ctx, f.Source(other), views(f, 5)); err != nil || n != 5 {
		t.Errorf("another device is held to the first one's quota: %d %v", n, err)
	}
	f.Clock.Advance(time.Hour)
	if n, _, err := svc.Ingest(ctx, src, views(f, 5)); err != nil || n != 5 {
		t.Errorf("the next hour: %d %v", n, err)
	}
}

// Verifies: ANL-010, LIM-001.
// A batch larger than telemetry.batchBytes is refused whole as PLX-8300,
// whether its size is measured on the wire or estimated, and stores nothing.
func TestIngestBatchBytes_ANL_010_LIM_001(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{Limits: tightened(t, limits.TelemetryBatchBytes, 400)})
	src := f.Source(f.Device)

	src.Bytes = 401
	if _, _, err := f.Telemetry.Ingest(ctx, src, views(f, 1)); code(err) != plxerr.IngestionOverQuota {
		t.Errorf("a batch of 401 bytes: %v", err)
	}
	src.Bytes = 400
	if n, _, err := f.Telemetry.Ingest(ctx, src, views(f, 1)); err != nil || n != 1 {
		t.Errorf("a batch of exactly the limit: %d %v", n, err)
	}
	src.Bytes = 0
	if _, _, err := f.Telemetry.Ingest(ctx, src, views(f, 30)); code(err) != plxerr.IngestionOverQuota {
		t.Errorf("a batch estimated over the limit: %v", err)
	}
	if got := f.Count(t, ""); got != 1 {
		t.Errorf("%d events stored, want the one accepted batch", got)
	}
}

// Verifies: ANL-010, SEC-161, ANL-003.
// The server re-checks consent: an event whose category the runtime did not
// declare consent for is refused one by one, operational events need none,
// the locale is dropped without analytics consent, and a runtime that
// declares nothing is taken at its word.
func TestIngestConsent_ANL_010_SEC_161(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{})
	now := f.Clock.Now()
	batch := []telemetry.Event{
		{Name: "session_start", Time: now, Fields: []byte(`{"runtime_version":"1.2.0","locale":"de-DE","device_class":"phone"}`)},
		{Name: "screen_view", Time: now, Route: "/loans"},
		{Name: "flag_evaluated", Time: now},
		{Name: "error", Time: now},
	}
	src := f.Source(f.Device)

	src.Consent = &telemetry.Consent{}
	n, diags, err := f.Telemetry.Ingest(ctx, src, batch)
	if err != nil || n != 2 || len(diags) != 2 || diags[0].Path != "/events/1" || diags[1].Path != "/events/2" {
		t.Fatalf("no consent: %d %v %v", n, diags, err)
	}
	if got := f.Count(t, "screen_view") + f.Count(t, "flag_evaluated"); got != 0 {
		t.Errorf("%d events of a refused category were stored", got)
	}
	var fields string
	if err := f.DB.InTx(ctx, storage.Tenant{OrganizationID: f.Org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT fields::text FROM analytics_events WHERE name = 'session_start'").Scan(&fields)
	}); err != nil || strings.Contains(fields, "locale") {
		t.Errorf("the locale was kept without analytics consent: %s (%v)", fields, err)
	}

	src.Consent = &telemetry.Consent{Analytics: true}
	if n, diags, _ := f.Telemetry.Ingest(ctx, src, batch[1:3]); n != 1 || len(diags) != 1 || diags[0].Path != "/events/1" {
		t.Errorf("analytics consent without experiments: %d %v", n, diags)
	}
	src.Consent = &telemetry.Consent{Analytics: true, Experiments: true}
	if n, diags, _ := f.Telemetry.Ingest(ctx, src, batch[1:3]); n != 2 || len(diags) != 0 {
		t.Errorf("both consents: %d %v", n, diags)
	}
	src.Consent = nil
	if n, diags, _ := f.Telemetry.Ingest(ctx, src, batch); n != 4 || len(diags) != 0 {
		t.Errorf("a runtime that declares nothing: %d %v", n, diags)
	}
}

// memStreams is an in-memory consumer-group stream with the delivery
// semantics of Valkey's: an entry goes to one consumer, stays pending until
// acknowledged, and may be claimed once it has been idle long enough.
type memStreams struct {
	mu      sync.Mutex
	now     func() time.Time
	entries []memEntry
	cursor  int
	pending map[string]memPending
	// failAcks is how many of the next acknowledgements fail, which is a
	// consumer that crashes after writing and before acknowledging.
	failAcks int
}

type memEntry struct {
	id      string
	payload []byte
}

type memPending struct {
	consumer string
	at       time.Time
}

func newMemStreams(now func() time.Time) *memStreams {
	return &memStreams{now: now, pending: map[string]memPending{}}
}

func (m *memStreams) StreamAdd(_ context.Context, _ string, _ int64, payload []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := fmt.Sprintf("%d-0", len(m.entries)+1)
	m.entries = append(m.entries, memEntry{id: id, payload: payload})
	return id, nil
}

func (*memStreams) StreamGroup(context.Context, string, string) error { return nil }

func (m *memStreams) StreamRead(_ context.Context, _, _, consumer string, count int) ([]cache.StreamEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []cache.StreamEntry
	for ; m.cursor < len(m.entries) && len(out) < count; m.cursor++ {
		e := m.entries[m.cursor]
		m.pending[e.id] = memPending{consumer: consumer, at: m.now()}
		out = append(out, cache.StreamEntry{ID: e.id, Payload: e.payload})
	}
	return out, nil
}

func (m *memStreams) StreamClaim(_ context.Context, _, _, consumer string, minIdle time.Duration, count int) ([]cache.StreamEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []cache.StreamEntry
	for _, e := range m.entries {
		p, ok := m.pending[e.id]
		if !ok || m.now().Sub(p.at) < minIdle || len(out) == count {
			continue
		}
		m.pending[e.id] = memPending{consumer: consumer, at: m.now()}
		out = append(out, cache.StreamEntry{ID: e.id, Payload: e.payload})
	}
	return out, nil
}

func (m *memStreams) StreamAck(_ context.Context, _, _ string, ids ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failAcks > 0 && len(ids) > 0 {
		m.failAcks--
		return errors.New("the connection to Valkey was lost")
	}
	for _, id := range ids {
		delete(m.pending, id)
	}
	return nil
}

func (m *memStreams) pendingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pending)
}

func consumer(streams cache.Streams, svc *telemetry.Service, name string) *telemetry.Consumer {
	return &telemetry.Consumer{Streams: streams, Service: svc, Name: name, ClaimAfter: time.Minute}
}

// Verifies: ANL-010.
// With a queue the endpoint stores nothing itself: a batch becomes events
// when a worker's consumer writes it, in one pass, and acknowledges it only
// afterwards. Events keep the dimensions their device's session_start gave.
func TestQueueConsumerWritesThenAcknowledges_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{})
	streams := newMemStreams(f.Clock.Now)
	svc := f.Ingestion(t, telemetry.Options{Queue: telemetry.StreamQueue{Streams: streams}})
	now := f.Clock.Now()
	batch := append([]telemetry.Event{{Name: "session_start", Time: now, Fields: []byte(`{"runtime_version":"1.2.0","device_class":"tablet","locale":"fr-FR"}`)}}, views(f, 3)...)

	if n, _, err := svc.Ingest(ctx, f.Source(f.Device), batch); err != nil || n != 4 {
		t.Fatalf("Ingest: %d %v", n, err)
	}
	if got := f.Count(t, ""); got != 0 {
		t.Fatalf("%d events were written before a consumer ran", got)
	}
	c := consumer(streams, svc, "worker-1")
	if handled, err := c.Once(ctx); err != nil || handled != 1 {
		t.Fatalf("Once: %d %v", handled, err)
	}
	if got := f.Count(t, ""); got != 4 || streams.pendingCount() != 0 {
		t.Errorf("%d events stored, %d batches pending; want 4 and 0", got, streams.pendingCount())
	}
	var class, locale string
	if err := f.DB.InTx(ctx, storage.Tenant{OrganizationID: f.Org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT device_class, locale FROM analytics_events WHERE name = 'screen_view' LIMIT 1").Scan(&class, &locale)
	}); err != nil || class != "tablet" || locale != "fr-FR" {
		t.Errorf("a screen view carries %q %q (%v)", class, locale, err)
	}
	if handled, err := c.Once(ctx); err != nil || handled != 0 {
		t.Errorf("an empty stream: %d %v", handled, err)
	}
}

// Verifies: ANL-010.
// A consumer that crashes between writing a batch and acknowledging it loses
// nothing and duplicates nothing: the batch is claimed again and its events,
// which carry the identifiers assigned on acceptance, are stored once.
func TestQueueCrashBetweenWriteAndAck_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{})
	streams := newMemStreams(f.Clock.Now)
	svc := f.Ingestion(t, telemetry.Options{Queue: telemetry.StreamQueue{Streams: streams}})
	if n, _, err := svc.Ingest(ctx, f.Source(f.Device), views(f, 4)); err != nil || n != 4 {
		t.Fatalf("Ingest: %d %v", n, err)
	}

	streams.failAcks = 1
	crashed := consumer(streams, svc, "worker-1")
	if _, err := crashed.Once(ctx); err == nil {
		t.Fatal("the lost acknowledgement was not reported")
	}
	if got := f.Count(t, ""); got != 4 || streams.pendingCount() != 1 {
		t.Fatalf("after the crash: %d events, %d pending; want 4 and 1", got, streams.pendingCount())
	}

	other := consumer(streams, svc, "worker-2")
	if handled, err := other.Once(ctx); err != nil || handled != 0 {
		t.Errorf("a batch claimed before it was stale: %d %v", handled, err)
	}
	f.Clock.Advance(2 * time.Minute)
	if handled, err := other.Once(ctx); err != nil || handled != 1 {
		t.Fatalf("the claim: %d %v", handled, err)
	}
	if got := f.Count(t, ""); got != 4 || streams.pendingCount() != 0 {
		t.Errorf("after the claim: %d events, %d pending; want 4 and 0 (no duplicate)", got, streams.pendingCount())
	}
}

// Verifies: ANL-010.
// A batch that a consumer read and never acknowledged, because its process
// died before writing, is taken over by another consumer once it has been
// idle for ClaimAfter, and written.
func TestQueueReclaimsStaleBatch_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{})
	streams := newMemStreams(f.Clock.Now)
	svc := f.Ingestion(t, telemetry.Options{Queue: telemetry.StreamQueue{Streams: streams}})
	if _, _, err := svc.Ingest(ctx, f.Source(f.Device), views(f, 2)); err != nil {
		t.Fatal(err)
	}
	if got, err := streams.StreamRead(ctx, telemetry.Stream, telemetry.Group, "dead-worker", 10); err != nil || len(got) != 1 {
		t.Fatalf("the dead worker read %v %v", got, err)
	}
	survivor := consumer(streams, svc, "worker-2")
	f.Clock.Advance(30 * time.Second)
	if handled, _ := survivor.Once(ctx); handled != 0 || f.Count(t, "") != 0 {
		t.Fatalf("a batch still in the hands of a live consumer was taken: %d", handled)
	}
	f.Clock.Advance(31 * time.Second)
	if handled, err := survivor.Once(ctx); err != nil || handled != 1 {
		t.Fatalf("the takeover: %d %v", handled, err)
	}
	if got := f.Count(t, ""); got != 2 || streams.pendingCount() != 0 {
		t.Errorf("%d events, %d pending; want 2 and 0", got, streams.pendingCount())
	}
}

// Verifies: ANL-010.
// A batch that can never be written is dropped and acknowledged rather than
// retried for ever: garbage in the stream, and a device deleted after its
// batch was accepted. A batch that merely failed stays pending.
func TestQueueDropsUnwritableBatches_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{})
	streams := newMemStreams(f.Clock.Now)
	svc := f.Ingestion(t, telemetry.Options{Queue: telemetry.StreamQueue{Streams: streams}})
	if _, err := streams.StreamAdd(ctx, telemetry.Stream, 0, []byte("not json")); err != nil {
		t.Fatal(err)
	}
	ghost := f.Source(f.Device)
	var err error
	if ghost.DeviceID, err = f.IDs.New(); err != nil {
		t.Fatal(err)
	}
	if n, _, err := svc.Ingest(ctx, ghost, views(f, 2)); err != nil || n != 2 {
		t.Fatalf("Ingest for a device the server does not know: %d %v", n, err)
	}
	c := consumer(streams, svc, "worker-1")
	if handled, err := c.Once(ctx); err != nil || handled != 2 {
		t.Fatalf("Once: %d %v", handled, err)
	}
	if streams.pendingCount() != 0 || f.Count(t, "") != 0 {
		t.Errorf("%d pending, %d events; want both dropped", streams.pendingCount(), f.Count(t, ""))
	}

	if n, _, err := svc.Ingest(ctx, f.Source(f.Device), views(f, 1)); err != nil || n != 1 {
		t.Fatal(err)
	}
	f.DB.Close()
	if _, err := c.Once(ctx); err == nil || streams.pendingCount() != 1 {
		t.Errorf("a failed write: %v, %d pending; want an error and the batch kept", err, streams.pendingCount())
	}
}

// failingQueue is a Valkey that is down.
type failingQueue struct{}

func (failingQueue) Enqueue(context.Context, []byte) error { return errors.New("valkey is down") }

// Verifies: ANL-010.
// Without a queue, or when the queue fails, the endpoint writes directly:
// telemetry is not lost because Valkey is down.
func TestIngestWritesDirectlyWithoutAWorkingQueue_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{})
	svc := f.Ingestion(t, telemetry.Options{Queue: failingQueue{}})
	if n, _, err := svc.Ingest(ctx, f.Source(f.Device), views(f, 3)); err != nil || n != 3 {
		t.Fatalf("Ingest: %d %v", n, err)
	}
	if got := f.Count(t, ""); got != 3 {
		t.Errorf("%d events stored, want 3", got)
	}
}

// Verifies: ANL-010.
// Delivering a batch twice stores each event once, keyed by the identifier
// given on acceptance.
func TestWriteIsIdempotentPerEvent_ANL_010(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := analyticstest.New(t, analyticstest.Options{})
	streams := newMemStreams(f.Clock.Now)
	svc := f.Ingestion(t, telemetry.Options{Queue: telemetry.StreamQueue{Streams: streams}})
	if _, _, err := svc.Ingest(ctx, f.Source(f.Device), views(f, 3)); err != nil {
		t.Fatal(err)
	}
	streams.mu.Lock()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(streams.entries[0].payload, &m); err != nil || len(m["events"]) == 0 {
		streams.mu.Unlock()
		t.Fatalf("the queued batch is %s (%v)", streams.entries[0].payload, err)
	}
	payload := streams.entries[0].payload
	streams.mu.Unlock()
	for range 3 {
		if _, err := streams.StreamAdd(ctx, telemetry.Stream, 0, payload); err != nil {
			t.Fatal(err)
		}
	}
	c := consumer(streams, svc, "worker-1")
	for range 2 {
		if _, err := c.Once(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.Count(t, ""); got != 3 {
		t.Errorf("%d events stored for one batch delivered four times, want 3", got)
	}
}
