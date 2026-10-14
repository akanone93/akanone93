// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/nightCode42/plux3/backend/internal/wasmrt"
)

// maxAddressable is the largest memory in which every pointer stays below
// 2^31, so that a packed (ptr << 32) | len is never negative.
const maxAddressable = 1 << 31

// Config configures an Engine. Clock, Random and MaxMemoryBytes are
// required: the package reads neither the wall clock nor the system's
// randomness on its own (CMP-002 style determinism for tests), and has no
// default for a limit (LIM-001).
type Config struct {
	// Clock reports the time guests see through clock_now and the WASI
	// clocks, and measures Stats.Duration.
	Clock func() time.Time
	// Random is the source of the WASI random_get; the engine serialises
	// reads, so any reader will do. Production passes crypto/rand.Reader.
	Random io.Reader
	// MaxMemoryBytes is the largest memory any invocation may be given
	// (`function.memory` maximum). Compile refuses a module whose declared
	// minimum is larger and runs plux_abi_version under this cap.
	MaxMemoryBytes uint64
	// CacheDir, when set, persists compiled code between processes so a
	// deployed module is compiled once (FN-010).
	CacheDir string
	// Interpreter runs modules in wazero's interpreter instead of its
	// compiler. The device runs functions in an interpreter (FN-050); the
	// CLI uses this engine as its stand-in, the shared ABI vectors keeping
	// both equivalent (FN-015).
	Interpreter bool
}

// Engine hosts function modules. It is safe for concurrent use: Compile and
// Invoke may be called from many goroutines.
type Engine struct {
	rt       wazero.Runtime
	cache    wazero.CompilationCache
	clock    func() time.Time
	random   io.Reader
	maxMem   uint64
	wasi     map[string]signature
	closeMu  sync.Once
	closeErr error
}

// NewEngine creates the runtime, instantiates WASI and the plux_v1 host
// module once. Close it when done.
func NewEngine(ctx context.Context, cfg Config) (*Engine, error) {
	if cfg.Clock == nil || cfg.Random == nil || cfg.MaxMemoryBytes == 0 {
		return nil, errors.New("fnhost.NewEngine: Clock, Random and MaxMemoryBytes are required")
	}
	if cfg.MaxMemoryBytes > maxAddressable {
		return nil, errors.New("fnhost.NewEngine: MaxMemoryBytes exceeds 2 GiB, the largest a packed pointer can address")
	}
	rc := wazero.NewRuntimeConfigCompiler()
	if cfg.Interpreter {
		rc = wazero.NewRuntimeConfigInterpreter()
	}
	rc = rc.WithCloseOnContextDone(true)
	e := &Engine{clock: cfg.Clock, random: &lockedReader{r: cfg.Random}, maxMem: cfg.MaxMemoryBytes}
	if cfg.CacheDir != "" {
		cache, err := wazero.NewCompilationCacheWithDir(cfg.CacheDir)
		if err != nil {
			return nil, fmt.Errorf("fnhost.NewEngine: compilation cache: %w", err)
		}
		e.cache = cache
		rc = rc.WithCompilationCache(cache)
	}
	e.rt = wasmrt.NewRuntime(ctx, rc)
	if err := e.instantiateHosts(ctx); err != nil {
		_ = e.Close(ctx)
		return nil, err
	}
	return e, nil
}

// instantiateHosts instantiates WASI and the plux_v1 module and records the
// WASI signatures a module may import.
func (e *Engine) instantiateHosts(ctx context.Context) error {
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, e.rt); err != nil {
		return fmt.Errorf("fnhost.NewEngine: WASI: %w", err)
	}
	e.wasi = map[string]signature{}
	for name, def := range e.rt.Module(wasiModule).ExportedFunctionDefinitions() {
		e.wasi[name] = signature{params: def.ParamTypes(), results: def.ResultTypes()}
	}
	if err := e.instantiatePluxV1(ctx); err != nil {
		return fmt.Errorf("fnhost.NewEngine: %s: %w", hostModule, err)
	}
	return nil
}

// Close releases the runtime, every module compiled on it and the cache.
// It is idempotent.
func (e *Engine) Close(ctx context.Context) error {
	e.closeMu.Do(func() {
		var errs []error
		if e.rt != nil {
			errs = append(errs, e.rt.Close(ctx))
		}
		if e.cache != nil {
			errs = append(errs, e.cache.Close(ctx))
		}
		if err := errors.Join(errs...); err != nil {
			e.closeErr = fmt.Errorf("fnhost.Engine.Close: %w", err)
		}
	})
	return e.closeErr
}

// lockedReader serialises reads so one source can serve concurrent
// instances.
type lockedReader struct {
	mu sync.Mutex
	r  io.Reader
}

// Read implements io.Reader.
func (l *lockedReader) Read(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.r.Read(p) //nolint:wrapcheck // a reader's errors are its own
}
