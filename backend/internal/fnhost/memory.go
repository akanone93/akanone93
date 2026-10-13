// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import (
	"github.com/tetratelabs/wazero/experimental"
)

// pageBytes is the size of a WebAssembly page.
const pageBytes = 65536

// cappedMemory is the experimental.MemoryAllocator of one instance
// (FN-011). It refuses to grow the linear memory beyond limit and records
// the refusal, so the trap that follows is reported as PLX-7002 rather
// than as a generic trap. It is used by one instance on one goroutine.
type cappedMemory struct {
	limit   uint64
	buf     []byte
	peak    uint64
	refused bool
}

// Allocate implements experimental.MemoryAllocator; it returns the one
// memory of the instance.
func (m *cappedMemory) Allocate(capacity, _ uint64) experimental.LinearMemory {
	m.buf = make([]byte, 0, min(capacity, m.limit))
	return m
}

// Reallocate implements experimental.LinearMemory. It returns nil, which
// wazero turns into a failed memory.grow, when size exceeds the limit. The
// bytes beyond the old length are zero because the memory only grows.
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

// Free implements experimental.LinearMemory.
func (m *cappedMemory) Free() { m.buf = nil }
