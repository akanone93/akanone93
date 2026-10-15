// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import (
	"sync"

	"github.com/tetratelabs/wazero/experimental"
)

// pageBytes is the size of a WebAssembly page.
const pageBytes = 65536

// PooledBufferMaxBytes is the largest linear memory the pool keeps for
// reuse. A typical Rust module starts with 1.1 MiB; a buffer above this
// size is left to the garbage collector, because zeroing it for reuse costs
// more than allocating a new one saves. It is a tuning constant of the
// allocator, not a limit on functions: a function may use any memory its
// `function.memory` limit allows (FN-011), pooled or not.
const PooledBufferMaxBytes = 4 << 20

// memoryPool keeps the linear memories of finished instances for the next
// ones, so that an invocation does not allocate its guest's memory afresh
// and hand it to the garbage collector a moment later (NFR-022). It is safe
// for concurrent use.
//
// Every buffer in the pool is entirely zero: release clears the bytes the
// instance could have written before it puts a buffer back, so no byte
// crosses from one invocation to the next (FN-010). A nil pool pools
// nothing.
type memoryPool struct {
	mu   sync.Mutex
	free [][]byte
	max  int
}

// newMemoryPool returns a pool that keeps at most max buffers, or nil when
// max is not positive.
func newMemoryPool(max int) *memoryPool {
	if max < 1 {
		return nil
	}
	return &memoryPool{max: max}
}

// take returns an empty buffer whose capacity is at least capacity, the
// smallest the pool holds, or nil.
func (p *memoryPool) take(capacity uint64) []byte {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	best := -1
	for i, b := range p.free {
		if uint64(cap(b)) >= capacity && (best < 0 || cap(b) < cap(p.free[best])) {
			best = i
		}
	}
	if best < 0 {
		return nil
	}
	b := p.free[best]
	last := len(p.free) - 1
	p.free[best], p.free[last] = p.free[last], nil
	p.free = p.free[:last]
	return b[:0]
}

// put clears the used part of buf and keeps it for the next instance. The
// bytes beyond len(buf) were never visible to the guest and are still zero.
// A buffer that is too large, or that finds the pool full, is dropped.
func (p *memoryPool) put(buf []byte) {
	if p == nil || cap(buf) == 0 || cap(buf) > PooledBufferMaxBytes {
		return
	}
	p.mu.Lock()
	full := len(p.free) >= p.max
	p.mu.Unlock()
	if full {
		return
	}
	clear(buf)
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.free) < p.max {
		p.free = append(p.free, buf[:0])
	}
}

// cappedMemory is the experimental.MemoryAllocator of one instance
// (FN-011). It refuses to grow the linear memory beyond limit and records
// the refusal, so the trap that follows is reported as PLX-7002 rather
// than as a generic trap. It is used by one instance on one goroutine, and
// draws its buffer from pool when there is one.
//
// The buffer returns to the pool in release, which the caller runs after
// the call has returned and the instance is closed, never in Free: wazero
// closes an instance that runs past its deadline from another goroutine
// while the guest is still executing, and a buffer handed on at that moment
// would be written by two instances.
type cappedMemory struct {
	limit   uint64
	pool    *memoryPool
	buf     []byte
	peak    uint64
	refused bool
}

// Allocate implements experimental.MemoryAllocator; it returns the one
// memory of the instance.
func (m *cappedMemory) Allocate(capacity, _ uint64) experimental.LinearMemory {
	want := min(capacity, m.limit)
	if b := m.pool.take(want); b != nil {
		m.buf = b
		return m
	}
	m.buf = make([]byte, 0, want)
	return m
}

// Reallocate implements experimental.LinearMemory. It returns nil, which
// wazero turns into a failed memory.grow, when size exceeds the limit. The
// bytes beyond the old length are zero because the memory only grows and a
// pooled buffer is cleared before it is pooled.
func (m *cappedMemory) Reallocate(size uint64) []byte {
	if size > m.limit {
		m.refused = true
		return nil
	}
	if size > uint64(cap(m.buf)) {
		grown := make([]byte, size, min(max(size, 2*uint64(cap(m.buf))), m.limit))
		copy(grown, m.buf)
		m.buf = grown
	} else {
		m.buf = m.buf[:size]
	}
	m.peak = max(m.peak, size)
	return m.buf
}

// Free implements experimental.LinearMemory. It keeps the buffer: see
// release.
func (*cappedMemory) Free() {}

// release gives the buffer back to the pool, cleared. The caller must have
// closed the instance and be sure the guest no longer runs.
func (m *cappedMemory) release() {
	buf := m.buf
	m.buf = nil
	m.pool.put(buf)
}
