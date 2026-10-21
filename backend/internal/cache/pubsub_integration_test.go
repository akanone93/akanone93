// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package cache_test

import (
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/cache/cachetest"
)

// eventually polls until the condition holds.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// receive returns the next message or fails.
func receive(t *testing.T, sub cache.Subscription) string {
	t.Helper()
	select {
	case m, ok := <-sub.Messages():
		if !ok {
			t.Fatal("the subscription closed")
		}
		return string(m)
	case <-time.After(10 * time.Second):
		t.Fatal("no message arrived")
	}
	return ""
}

// TestMemoryNotifierDeliversToEverySubscriber covers the single-node
// backend of the change notices that GOV-007 pushes.
//
// Verifies: GOV-007.
func TestMemoryNotifierDeliversToEverySubscriber(t *testing.T) {
	m := cache.NewMemory(nil)
	a, err := m.Subscribe(t.Context(), "c")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := m.Subscribe(t.Context(), "c")
	other, _ := m.Subscribe(t.Context(), "other")
	if err := m.Publish(t.Context(), "c", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if receive(t, a) != "hello" || receive(t, b) != "hello" {
		t.Error("a subscriber missed the message")
	}
	select {
	case got := <-other.Messages():
		t.Errorf("another channel received %q", got)
	default:
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-a.Messages(); ok {
		t.Error("a closed subscription still delivers")
	}
	if live, _ := a.State(); live {
		t.Error("a closed subscription is live")
	}
}

// TestMemoryNotifierRaisesTheGenerationWhenItDrops shows that a reader too
// slow to keep up learns that it missed a message.
//
// Verifies: GOV-007.
func TestMemoryNotifierRaisesTheGenerationWhenItDrops(t *testing.T) {
	m := cache.NewMemory(nil)
	sub, _ := m.Subscribe(t.Context(), "c")
	_, before := sub.State()
	for range 200 {
		if err := m.Publish(t.Context(), "c", []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if _, after := sub.State(); after == before {
		t.Error("the generation did not rise although messages were dropped")
	}
}

// TestMemoryNotifierEndsWithTheCache shows that closing the cache ends its
// subscriptions and refuses new ones.
//
// Verifies: GOV-007.
func TestMemoryNotifierEndsWithTheCache(t *testing.T) {
	m := cache.NewMemory(nil)
	sub, _ := m.Subscribe(t.Context(), "c")
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-sub.Messages(); ok {
		t.Error("the subscription outlived the cache")
	}
	if _, err := m.Subscribe(t.Context(), "c"); err == nil {
		t.Error("a closed cache accepted a subscription")
	}
}

// TestValkeyNotifierCrossesReplicas shows a message published through one
// client reach a subscription of another, and that the subscription
// recovers from an outage with a new generation.
//
// Verifies: GOV-007.
func TestValkeyNotifierCrossesReplicas(t *testing.T) {
	server := cachetest.Start(t)
	pub, err := cache.NewValkey(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pub.Close() })
	other, err := cache.NewValkey(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close() })
	sub, err := other.Subscribe(t.Context(), "c")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Close() })
	eventually(t, "the subscription to go live", func() bool { live, _ := sub.State(); return live })
	_, first := sub.State()

	if err := pub.Publish(t.Context(), "c", []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, sub); got != "v1" {
		t.Errorf("received %q", got)
	}

	server.Stop()
	eventually(t, "the subscription to notice the outage", func() bool { live, _ := sub.State(); return !live })
	server.Restart()
	eventually(t, "the subscription to recover", func() bool { live, _ := sub.State(); return live })
	if _, again := sub.State(); again <= first {
		t.Errorf("generation %d after the outage, %d before: messages lost meanwhile would go unnoticed", again, first)
	}
	// The publisher's pooled connections died with the server; the first
	// call finds that out and later ones reconnect.
	eventually(t, "a publish to succeed again", func() bool {
		return pub.Publish(t.Context(), "c", []byte("v2")) == nil
	})
	if got := receive(t, sub); got != "v2" {
		t.Errorf("received %q after the outage", got)
	}
}

// TestValkeySubscriptionClosesPromptly shows Close ends the goroutine even
// while the server is unreachable.
//
// Verifies: GOV-007.
func TestValkeySubscriptionClosesPromptly(t *testing.T) {
	server := cachetest.Start(t)
	v, err := cache.NewValkey(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = v.Close() })
	server.Stop()
	sub, err := v.Subscribe(t.Context(), "c")
	if err != nil {
		t.Fatal(err)
	}
	if live, _ := sub.State(); live {
		t.Error("a subscription to a stopped server is live")
	}
	if err := sub.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-sub.Messages(); ok {
		t.Error("a closed subscription still delivers")
	}
}
