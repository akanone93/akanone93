// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package statuslist

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/cache"
)

const (
	listOne = `{"entries":{"c35747a084470c3135aeefe2b8d40921":{"status":"REVOKED","reason":"KEY_COMPROMISE"}}}`
	listTwo = `{"entries":{"c35747a084470c3135aeefe2b8d40921":{"status":"REVOKED"},"0a":{"status":"SUSPENDED"}}}`
)

// clock is a settable clock.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newClock() *clock { return &clock{t: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)} }

// server serves the body and counts the requests; it answers 304 to a
// matching If-None-Match.
type server struct {
	*httptest.Server
	mu           sync.Mutex
	body         string
	status       int
	cacheControl string
	requests     int
	conditional  int
}

func newServer(t *testing.T) *server {
	t.Helper()
	s := &server{body: listOne, status: http.StatusOK, cacheControl: "public, max-age=3600"}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.requests++
		w.Header().Set("Cache-Control", s.cacheControl)
		if s.status != http.StatusOK {
			w.WriteHeader(s.status)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("If-None-Match") == `"v1"` {
			s.conditional++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte(s.body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *server) set(f func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f()
}

func fetchList(t *testing.T, s *server, shared cache.Cache, c *clock, maxBytes int64) *List {
	t.Helper()
	l, err := New(Options{URL: s.URL, Client: s.Client(), Cache: shared, MaxBytes: maxBytes, Now: c.now})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// Verifies: SEC-002.
// The worker's refresh publishes the list Google serves; an api replica
// that never fetched reads it from the shared cache.
func TestRefreshPublishesToTheSharedCache(t *testing.T) {
	c := newClock()
	shared := cache.NewMemory(c.now)
	srv := newServer(t)
	worker := fetchList(t, srv, shared, c, 1<<20)
	replica := fetchList(t, srv, shared, c, 1<<20)
	ctx := context.Background()

	if _, err := replica.Revocations(ctx); err == nil {
		t.Fatal("a replica answered before any list was fetched")
	}
	if err := worker.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	got, err := replica.Revocations(ctx)
	if err != nil || got.Len() != 1 {
		t.Fatalf("replica: %v entries, %v", got.Len(), err)
	}
}

// Verifies: SEC-002.
// Within the freshness the response allows, Refresh does not ask again; it
// asks conditionally once half of it has passed, and a 304 keeps the list.
func TestRefreshHonoursTheHTTPCacheHeaders(t *testing.T) {
	c := newClock()
	shared := cache.NewMemory(c.now)
	srv := newServer(t)
	l := fetchList(t, srv, shared, c, 1<<20)
	ctx := context.Background()

	for range 3 {
		if err := l.Refresh(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if srv.requests != 1 {
		t.Fatalf("requests within the freshness = %d, want 1", srv.requests)
	}
	c.advance(31 * time.Minute)
	if err := l.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if srv.requests != 2 || srv.conditional != 1 {
		t.Fatalf("requests = %d, conditional = %d; want 2 and 1", srv.requests, srv.conditional)
	}
	if got, err := l.Revocations(ctx); err != nil || got.Len() != 1 {
		t.Fatalf("after 304: %v, %v", got.Len(), err)
	}
}

// Verifies: SEC-002.
// A cache that lost the list is repopulated at the next refresh, although
// the list was not yet due.
func TestRefreshRepopulatesALostList(t *testing.T) {
	c := newClock()
	shared := cache.NewMemory(c.now)
	srv := newServer(t)
	l := fetchList(t, srv, shared, c, 1<<20)
	ctx := context.Background()
	if err := l.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if err := shared.Delete(ctx, cacheKey); err != nil {
		t.Fatal(err)
	}
	if err := l.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := shared.Get(ctx, cacheKey); !ok {
		t.Fatal("the list was not published again")
	}
}

// Verifies: SEC-002, SEC-009.
// The list expires with its freshness: a replica then has none, which the
// verifier reports as the provider's outage.
func TestAStaleListIsUnavailable(t *testing.T) {
	c := newClock()
	shared := cache.NewMemory(c.now)
	srv := newServer(t)
	l := fetchList(t, srv, shared, c, 1<<20)
	ctx := context.Background()
	if err := l.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Revocations(ctx); err != nil {
		t.Fatal(err)
	}
	c.advance(time.Hour + time.Minute)
	if _, err := l.Revocations(ctx); err == nil {
		t.Fatal("a list past its freshness was still served")
	}
}

// Verifies: SEC-002.
func TestRefreshRefusesWhatIsNotAList(t *testing.T) {
	ctx := context.Background()
	for name, tune := range map[string]func(*server){
		"server error":   func(s *server) { s.status = http.StatusInternalServerError },
		"not JSON":       func(s *server) { s.body = "<html>" },
		"no entries":     func(s *server) { s.body = "{}" },
		"too large":      func(s *server) { s.body = listTwo + "                                        " },
		"not authorised": func(s *server) { s.status = http.StatusForbidden },
	} {
		t.Run(name, func(t *testing.T) {
			c := newClock()
			srv := newServer(t)
			srv.set(func() { tune(srv) })
			l := fetchList(t, srv, cache.NewMemory(c.now), c, int64(len(listTwo)))
			if err := l.Refresh(ctx); err == nil {
				t.Fatal("Refresh accepted it")
			}
			if _, err := l.Revocations(ctx); err == nil {
				t.Fatal("a list appeared although nothing valid was fetched")
			}
		})
	}
}

// Verifies: SEC-002.
// A failed refresh leaves the list that is still fresh in the cache.
func TestAFailedRefreshKeepsTheFreshList(t *testing.T) {
	c := newClock()
	shared := cache.NewMemory(c.now)
	srv := newServer(t)
	l := fetchList(t, srv, shared, c, 1<<20)
	ctx := context.Background()
	if err := l.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	srv.set(func() { srv.status = http.StatusBadGateway })
	c.advance(45 * time.Minute)
	if err := l.Refresh(ctx); err == nil {
		t.Fatal("a failing server was not reported")
	}
	if got, err := l.Revocations(ctx); err != nil || got.Len() != 1 {
		t.Fatalf("the fresh list was lost: %v, %v", got.Len(), err)
	}
}

// Verifies: SEC-002.
func TestFreshnessIsClamped(t *testing.T) {
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		header http.Header
		want   time.Duration
	}{
		{"none", http.Header{}, time.Hour},
		{"max-age", http.Header{"Cache-Control": {"public, max-age=7200"}}, 2 * time.Hour},
		{"max-age zero", http.Header{"Cache-Control": {"max-age=0"}}, minLifetime},
		{"max-age year", http.Header{"Cache-Control": {"max-age=31536000"}}, maxLifetime},
		{"max-age garbage", http.Header{"Cache-Control": {"max-age=soon"}}, time.Hour},
		{"expires", http.Header{"Expires": {now.Add(3 * time.Hour).Format(http.TimeFormat)}}, 3 * time.Hour},
		{"expires past", http.Header{"Expires": {now.Add(-time.Hour).Format(http.TimeFormat)}}, minLifetime},
	}
	for _, tt := range tests {
		if got := freshFor(tt.header, now); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

// Verifies: SEC-002, DEP-040.
// An operator's file replaces the fetch, is re-read when it changes, and
// keeps the last good list while it is unreadable.
func TestOperatorFile(t *testing.T) {
	c := newClock()
	path := filepath.Join(t.TempDir(), "status.json")
	if err := os.WriteFile(path, []byte(listOne), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := New(Options{File: path, MaxBytes: 1 << 20, Now: c.now})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if !l.FromFile() || l.Refresh(ctx) != nil {
		t.Fatal("a file list is refreshed in the background")
	}
	if got, err := l.Revocations(ctx); err != nil || got.Len() != 1 {
		t.Fatalf("first read: %v, %v", got.Len(), err)
	}

	if err := os.WriteFile(path, []byte(listTwo), 0o600); err != nil {
		t.Fatal(err)
	}
	c.advance(time.Minute)
	if got, err := l.Revocations(ctx); err != nil || got.Len() != 2 {
		t.Fatalf("after the change: %v, %v", got.Len(), err)
	}

	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	c.advance(time.Minute)
	if got, err := l.Revocations(ctx); err != nil || got.Len() != 2 {
		t.Fatalf("a broken file dropped the last good list: %v, %v", got.Len(), err)
	}
}

// Verifies: SEC-002, SEC-009.
// A file that was never readable is an outage, not an empty list.
func TestMissingOrOversizedFileIsUnavailable(t *testing.T) {
	c := newClock()
	dir := t.TempDir()
	ctx := context.Background()
	missing, err := New(Options{File: filepath.Join(dir, "none.json"), MaxBytes: 1 << 20, Now: c.now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := missing.Revocations(ctx); err == nil {
		t.Fatal("a missing file gave a list")
	}
	big := filepath.Join(dir, "big.json")
	if err := os.WriteFile(big, []byte(listTwo), 0o600); err != nil {
		t.Fatal(err)
	}
	small, err := New(Options{File: big, MaxBytes: 16, Now: c.now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := small.Revocations(ctx); err == nil {
		t.Fatal("a file over the bound gave a list")
	}
}

func TestNewValidatesItsOptions(t *testing.T) {
	c := cache.NewMemory(nil)
	for name, o := range map[string]Options{
		"no bound":     {File: "x"},
		"no client":    {Cache: c, MaxBytes: 1},
		"no cache":     {Client: http.DefaultClient, MaxBytes: 1},
		"plain http":   {URL: "http://android.googleapis.com/attestation/status", Client: http.DefaultClient, Cache: c, MaxBytes: 1},
		"unsafe other": {URL: "file:///etc/passwd", Client: http.DefaultClient, Cache: c, MaxBytes: 1},
	} {
		if _, err := New(o); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := New(Options{Client: http.DefaultClient, Cache: c, MaxBytes: 1}); err != nil {
		t.Errorf("the default URL: %v", err)
	}
}
