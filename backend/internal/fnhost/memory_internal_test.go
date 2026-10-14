// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// data returns the address of a buffer's backing array.
func data(b []byte) *byte { return &b[:1][0] }

func allZero(b []byte) bool {
	return !bytes.ContainsFunc(b[:cap(b)], func(r rune) bool { return r != 0 })
}

// Verifies: FN-010, FN-011.
// A second instance that draws the first one's buffer from the pool finds it
// cleared whole, and the memory limit holds whatever the buffer's capacity.
func TestPooledMemoryIsClearedBetweenInstances_FN_010(t *testing.T) {
	t.Parallel()
	pool := newMemoryPool(2)
	first := &cappedMemory{limit: 8 << 20, pool: pool}
	lm := first.Allocate(1<<20, 0)
	mem := lm.Reallocate(1 << 20)
	if len(mem) != 1<<20 {
		t.Fatalf("len = %d", len(mem))
	}
	for i := range mem {
		mem[i] = 0xAA
	}
	grown := lm.Reallocate(3 << 20) // growth beyond the capacity replaces the buffer
	for i := range grown {
		grown[i] = 0xBB
	}
	lm.Free()
	if first.buf == nil {
		t.Fatal("Free dropped the buffer, which a guest still running past its deadline could write to")
	}
	address := data(first.buf)
	first.release()

	second := &cappedMemory{limit: 1 << 20, pool: pool}
	lm = second.Allocate(1<<20, 0)
	if data(second.buf) != address {
		t.Fatal("the second instance did not reuse the first one's buffer; the test proves nothing")
	}
	// The limit is below the pooled buffer's capacity.
	if got := lm.Reallocate(1 << 20); len(got) != 1<<20 || !allZero(got) {
		t.Fatalf("the reused memory holds the previous instance's bytes (len %d)", len(got))
	}
	if lm.Reallocate(1<<20+1) != nil || !second.refused {
		t.Error("a pooled buffer larger than the limit let the memory grow past it")
	}
	if second.peak > second.limit {
		t.Errorf("peak = %d above the limit %d", second.peak, second.limit)
	}
}

// The pool is bounded in count and in the size of what it keeps, hands out
// the smallest buffer that fits, and a nil pool pools nothing.
func TestMemoryPoolBounds(t *testing.T) {
	t.Parallel()
	pool := newMemoryPool(2)
	pool.put(make([]byte, 0, PooledBufferMaxBytes+1))
	pool.put(nil)
	if b := pool.take(1); b != nil {
		t.Fatal("the pool kept a buffer above PooledBufferMaxBytes or an empty one")
	}
	small, large, extra := make([]byte, 100, 1<<16), make([]byte, 100, 1<<20), make([]byte, 100, 1<<17)
	pool.put(large)
	pool.put(small)
	pool.put(extra) // the pool is full
	if len(pool.free) != 2 {
		t.Fatalf("the pool holds %d buffers, want 2", len(pool.free))
	}
	if b := pool.take(1 << 15); b == nil || cap(b) != 1<<16 || len(b) != 0 {
		t.Errorf("take(32 KiB) = cap %d, want the smallest that fits, 64 KiB", cap(b))
	}
	if b := pool.take(1 << 21); b != nil {
		t.Error("take returned a buffer smaller than asked")
	}
	if !allZero(large) || !allZero(small) {
		t.Error("a pooled buffer was not cleared")
	}

	var none *memoryPool
	none.put(make([]byte, 10))
	if none.take(1) != nil || newMemoryPool(0) != nil {
		t.Error("a nil pool pooled something")
	}
	m := &cappedMemory{limit: 1 << 20}
	m.Allocate(1<<16, 0).Reallocate(1 << 16)
	m.release() // must not panic without a pool
}

// probeEngine returns an engine over the conformance probe with a pool of
// size buffers.
func probeEngine(t *testing.T, buffers int) (*Engine, *Module) {
	t.Helper()
	ctx := context.Background()
	e, err := NewEngine(ctx, Config{
		Clock: func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, Random: bytes.NewReader(make([]byte, 1<<16)),
		MaxMemoryBytes: 512 << 20, MemoryPoolBuffers: buffers,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close(ctx) })
	wasm, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", "testdata", "functions", "probe.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := e.Compile(ctx, wasm, PlacementServer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close(ctx) })
	return e, m
}

func probeLimits() Limits {
	return Limits{Timeout: 10 * time.Second, MemoryBytes: 64 << 20, InputBytes: 1 << 20, OutputBytes: 1 << 20, LogBytes: 4096, HTTPCalls: 10, CallDepth: 3}
}

// Verifies: FN-010, FN-011.
// Through a real engine: the instances reuse the pool's buffers, each
// starts from the probe's own initial state, and whatever a call left in
// its memory (a large input, a trap, a timeout, a refused growth) is gone
// from every buffer the pool holds, concurrent calls included.
func TestEnginePoolsGuestMemory_FN_010(t *testing.T) {
	t.Parallel()
	e, m := probeEngine(t, 4)
	ctx := context.Background()

	secret := bytes.Repeat([]byte{0xA5}, 128<<10)
	out, _, err := e.Invoke(ctx, m, Call{Index: 0, Input: secret, Limits: probeLimits()})
	if err != nil || !bytes.Equal(out.Output, secret) {
		t.Fatalf("echo: %v", err)
	}
	if len(e.pool.free) != 1 {
		t.Fatalf("the pool holds %d buffers after one call, want 1", len(e.pool.free))
	}
	address := data(e.pool.free[0])
	if !allZero(e.pool.free[0]) {
		t.Fatal("the buffer went back to the pool with the previous input in it")
	}
	out, _, err = e.Invoke(ctx, m, Call{Index: 18, Limits: probeLimits()})
	if err != nil || string(out.Output) != `{"counter":1}` {
		t.Fatalf("counter = %s, %v", out.Output, err)
	}
	if len(e.pool.free) != 1 || data(e.pool.free[0]) != address {
		t.Error("the second call did not reuse the pooled buffer")
	}

	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			for j := range 4 {
				input := fmt.Sprintf(`{"caller":%d,"round":%d}`, i, j)
				switch (i + j) % 4 {
				case 0:
					o, _, err := e.Invoke(ctx, m, Call{Index: 0, Input: []byte(input), Limits: probeLimits()})
					if err != nil || string(o.Output) != input {
						t.Errorf("echo = %q, %v; want %q", o.Output, err, input)
					}
				case 1:
					_, _, err := e.Invoke(ctx, m, Call{Index: 11, Limits: probeLimits()}) // a trap
					if err == nil {
						t.Error("the panic entry succeeded")
					}
				case 2:
					l := probeLimits()
					l.Timeout = 30 * time.Millisecond
					if _, _, err := e.Invoke(ctx, m, Call{Index: 8, Limits: l}); err == nil { // an endless loop
						t.Error("the loop ended without a timeout")
					}
				default:
					l := probeLimits()
					l.MemoryBytes = 6 << 20
					if _, _, err := e.Invoke(ctx, m, Call{Index: 9, Limits: l}); err == nil { // growth until refused
						t.Error("the growing entry succeeded")
					}
				}
			}
		})
	}
	wg.Wait()
	if len(e.pool.free) == 0 || len(e.pool.free) > 4 {
		t.Errorf("the pool holds %d buffers, want between 1 and 4", len(e.pool.free))
	}
	for i, b := range e.pool.free {
		if !allZero(b) {
			t.Errorf("pooled buffer %d holds bytes of an earlier call", i)
		}
	}
}
