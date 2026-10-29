// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/cache/cachetest"
)

// TestValkeyFollowCrossesReplicas_DEV_021_STU_075 shows a follower on one
// client receive, in order, what a writer on another client appended, and
// resume after the last entry it saw.
func TestValkeyFollowCrossesReplicas_DEV_021_STU_075(t *testing.T) {
	server := cachetest.Start(t)
	wc, err := cache.NewValkey(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wc.Close() })
	fc, err := cache.NewValkey(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fc.Close() })
	w := cache.LogWriter{Logs: wc, Notifier: wc, MaxLen: 100, TTL: time.Minute}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	got := make(chan cache.StreamEntry, 8)
	done := make(chan error, 1)
	go func() {
		done <- cache.Follow(ctx, fc, fc, "it", "", 50*time.Millisecond, 2, func(e cache.StreamEntry) error {
			got <- e
			return nil
		})
	}()
	for _, p := range []string{"a", "b", "c"} {
		if _, err := w.Append(ctx, "it", []byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	var last string
	for _, want := range []string{"a", "b", "c"} {
		select {
		case e := <-got:
			if string(e.Payload) != want {
				t.Fatalf("got %q, want %q", e.Payload, want)
			}
			last = e.ID
		case <-time.After(10 * time.Second):
			t.Fatalf("no entry %q arrived", want)
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("Follow returned %v; want context.Canceled", err)
	}
	if _, err := w.Append(t.Context(), "it", []byte("d")); err != nil {
		t.Fatal(err)
	}
	entries, err := fc.LogRead(t.Context(), "it", last, 10)
	if err != nil || len(entries) != 1 || string(entries[0].Payload) != "d" {
		t.Errorf("resume after %s = %v, %v; want [d]", last, entries, err)
	}
}
