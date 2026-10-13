// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"context"
	"sync"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
)

// Verifies: FN-010.
// A compiled module is cached: the second invocation does not fetch, and
// Prepare warms the cache so the first invocation does not either.
func TestModuleCacheHitsAndPrepare_FN_010(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	ctx := context.Background()

	if _, err := h.runner.Prepare(ctx, connect.NewRequest(&fnpb.PrepareRequest{ModuleSha256: h.probeDigest(), Grant: "prep-grant"})); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if got := h.api.calls("GetModule"); len(got) != 1 || got[0] != "prep-grant" {
		t.Fatalf("GetModule grants = %v, want [prep-grant]", got)
	}
	for range 3 {
		if _, err := h.call(0, h.request(`{"a":1}`)); err != nil {
			t.Fatalf("Invoke: %v", err)
		}
	}
	if got := len(h.api.calls("GetModule")); got != 1 {
		t.Errorf("GetModule called %d times, want 1", got)
	}
	if hit, miss := h.reg.counter(t, "plux_function_module_cache_total", cacheHit), h.reg.counter(t, "plux_function_module_cache_total", cacheMiss); hit != 3 || miss != 1 {
		t.Errorf("cache hit/miss = %v/%v, want 3/1", hit, miss)
	}
}

// Verifies: FN-010.
// A cache miss on Invoke fetches with the request's grant, and a module the
// api role does not hold is PLX-7012.
func TestModuleCacheMissFetchesWithTheGrant_FN_010(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	if _, err := h.call(0, h.request("{}")); err != nil {
		t.Fatal(err)
	}
	if got := h.api.calls("GetModule"); len(got) != 1 || got[0] != "grant-1" {
		t.Errorf("GetModule grants = %v", got)
	}
	msg := h.request("{}")
	msg.ModuleSha256 = digest([]byte("unknown"))
	if _, err := h.call(0, msg); plxCode(t, err) != "PLX-7012" {
		t.Errorf("unknown module: %v", err)
	}
	msg.ModuleSha256 = "NOT-A-DIGEST"
	if _, err := h.call(0, msg); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("malformed digest: %v", err)
	}
	if _, err := h.runner.Prepare(context.Background(), connect.NewRequest(&fnpb.PrepareRequest{ModuleSha256: "x"})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("Prepare with a malformed digest: %v", err)
	}
}

// Verifies: FN-010.
// The cache is bounded in modules: the least recently used one is evicted
// and fetched again on its next use.
func TestModuleCacheEvictsByCount_FN_010(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{tweak: func(o *Options) { o.MaxModules = 1 }})
	a := h.request("{}")
	b := h.request("{}")
	b.ModuleSha256 = digest(probe(t, "probe-device.wasm"))

	for _, msg := range []*fnpb.FunctionRunnerServiceInvokeRequest{a, b, b, a} {
		if _, err := h.call(0, msg); err != nil {
			t.Fatalf("Invoke: %v", err)
		}
	}
	if got := len(h.api.calls("GetModule")); got != 3 {
		t.Errorf("GetModule called %d times, want 3 (a, b, a again)", got)
	}
	if n, _ := h.runner.cache.len(); n != 1 {
		t.Errorf("%d modules cached, want 1", n)
	}
}

// Verifies: FN-010.
// The cache is bounded in bytes: with room for one probe, the next module
// evicts the first. A module above the bound is refused as invalid.
func TestModuleCacheBoundsBytes_FN_010(t *testing.T) {
	t.Parallel()
	good := probe(t, "probe.wasm")
	budget := int64(len(good)) + 10
	h := newHarness(t, harnessOptions{tweak: func(o *Options) { o.MaxModuleBytes = budget }})
	b := h.request("{}")
	b.ModuleSha256 = digest(probe(t, "probe-device.wasm"))

	for _, msg := range []*fnpb.FunctionRunnerServiceInvokeRequest{h.request("{}"), b} {
		if _, err := h.call(0, msg); err != nil {
			t.Fatal(err)
		}
	}
	if n, size := h.runner.cache.len(); size > budget || n != 1 {
		t.Errorf("cache holds %d modules, %d bytes, budget %d", n, size, budget)
	}

	tight := newHarness(t, harnessOptions{tweak: func(o *Options) { o.MaxModuleBytes = int64(len(good)) - 1 }})
	if _, err := tight.call(0, tight.request("{}")); plxCode(t, err) != "PLX-7018" {
		t.Errorf("a module above the bound: %v", err)
	}
}

// Verifies: FN-010.
// A module evicted while an invocation still holds it is closed once, by the
// last release, never under the running invocation.
func TestEvictedModuleClosesAfterLastRelease_FN_010(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{tweak: func(o *Options) { o.MaxModules = 1 }})
	ctx := context.Background()
	held, err := h.runner.module(ctx, h.probeDigest(), "g")
	if err != nil {
		t.Fatal(err)
	}
	other := digest(probe(t, "probe-device.wasm"))
	evictor, err := h.runner.module(ctx, other, "g")
	if err != nil {
		t.Fatal(err)
	}
	h.runner.cache.release(ctx, evictor)

	h.runner.cache.mu.Lock()
	evicted, shut := held.evicted, held.shut
	h.runner.cache.mu.Unlock()
	if !evicted || shut {
		t.Fatalf("held module: evicted=%v shut=%v, want evicted and open", evicted, shut)
	}
	h.runner.cache.release(ctx, held)
	h.runner.cache.mu.Lock()
	shut = held.shut
	h.runner.cache.mu.Unlock()
	if !shut {
		t.Error("the evicted module was not closed at its last release")
	}
	h.runner.cache.closeIfIdle(ctx, held) // a second close is a no-op
}

// Verifies: FN-010.
// Concurrent first uses of one module share a single fetch.
func TestConcurrentMissesShareOneFetch_FN_010(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	gate := make(chan struct{})
	h.api.mu.Lock()
	h.api.gate = gate
	h.api.mu.Unlock()

	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.call(0, h.request("{}"))
			errs <- err
		}()
	}
	eventually(t, "the leader to ask for the module", func() bool { return len(h.api.calls("GetModule")) == 1 })
	close(gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Invoke: %v", err)
		}
	}
	if got := len(h.api.calls("GetModule")); got != 1 {
		t.Errorf("GetModule called %d times, want 1", got)
	}
}

// Verifies: FN-010.
// A waiter whose own request ends stops waiting with PLX-7021.
func TestWaiterStopsWhenItsRequestEnds_FN_010(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	gate := make(chan struct{})
	h.api.mu.Lock()
	h.api.gate = gate
	h.api.mu.Unlock()
	defer close(gate)

	go func() { _, _ = h.call(0, h.request("{}")) }()
	eventually(t, "the leader to ask for the module", func() bool { return len(h.api.calls("GetModule")) == 1 })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := h.runner.invoke(ctx, h.request("{}"), 0)
	if plxCode(t, err) != "PLX-7021" {
		t.Errorf("waiter: %v", err)
	}
}
