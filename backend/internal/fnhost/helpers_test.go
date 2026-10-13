// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/fnhost"
)

// testClock is the instant every test guest sees: 2023-11-14T22:13:20Z.
var testClock = time.Unix(1_700_000_000, 0).UTC()

// counterReader yields 0, 1, 2, … so a random draw is predictable.
type counterReader struct {
	mu   sync.Mutex
	next byte
}

func (c *counterReader) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range p {
		p[i] = c.next
		c.next++
	}
	return len(p), nil
}

// newEngine returns an engine with the fixed clock and the counter source.
func newEngine(t testing.TB) *fnhost.Engine {
	t.Helper()
	ctx := context.Background()
	e, err := fnhost.NewEngine(ctx, fnhost.Config{
		Clock:          func() time.Time { return testClock },
		Random:         &counterReader{},
		MaxMemoryBytes: 512 << 20,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	t.Cleanup(func() { _ = e.Close(ctx) })
	return e
}

// fixture reads a file of schema/testdata/functions.
func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", "testdata", "functions", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

// compile compiles a probe module for a placement.
func compile(t testing.TB, e *fnhost.Engine, file string, p fnhost.Placement) *fnhost.Module {
	t.Helper()
	m, err := e.Compile(context.Background(), fixture(t, file), p)
	if err != nil {
		t.Fatalf("Compile %s: %v", file, err)
	}
	t.Cleanup(func() { _ = m.Close(context.Background()) })
	return m
}

// defaultLimits are generous limits for tests that exercise something else.
func defaultLimits() fnhost.Limits {
	return fnhost.Limits{
		Timeout: 10 * time.Second, MemoryBytes: 64 << 20, InputBytes: 1 << 20,
		OutputBytes: 1 << 20, LogBytes: 4096, HTTPCalls: 10, CallDepth: 3,
	}
}
