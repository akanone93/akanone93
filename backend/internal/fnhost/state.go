// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import (
	"bytes"
	"context"
	"errors"
	"math"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/sys"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// exitFault is the exit code with which a host call ends an instance after
// it recorded a fault. wazero reserves 0xffffffff and 0xefffffff for
// cancellation and deadline.
const exitFault uint32 = 0xfa17

// callState is the per-call state the host functions read from the call's
// context. One call runs on one goroutine, so it needs no locking.
type callState struct {
	engine    *Engine
	module    *Module
	call      *Call
	memory    *cappedMemory
	budget    *logBudget
	ctxJSON   []byte
	httpCalls int
	// fault is why a host function ended the call; classify reports it.
	fault error
	// denied records that a capability was refused (FN-012), so that a trap
	// which follows is reported as PLX-7003 rather than a generic trap.
	denied bool
}

// deny records a refused capability and returns the status the guest sees.
func (st *callState) deny() int32 {
	st.denied = true
	return statusDenied
}

type stateKey struct{}

// attach returns a context that carries the state to the host functions and
// the memory allocator to the instantiation.
func (st *callState) attach(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, stateKey{}, st)
	return experimental.WithMemoryAllocator(ctx, st.memory)
}

// unavailableHost serves nothing; it stands in for a nil Host.
type unavailableHost struct{}

func (unavailableHost) KVGet(context.Context, string) ([]byte, error) { return nil, ErrUnavailable }
func (unavailableHost) KVPut(context.Context, string, []byte) error   { return ErrUnavailable }
func (unavailableHost) KVDelete(context.Context, string) error        { return ErrUnavailable }
func (unavailableHost) HTTP(context.Context, HTTPRequest) (HTTPResponse, error) {
	return HTTPResponse{}, ErrUnavailable
}

func (unavailableHost) Secret(context.Context, string) ([]byte, error) { return nil, ErrUnavailable }

func (unavailableHost) Invoke(context.Context, string, []byte, int) (Outcome, error) {
	return Outcome{}, ErrUnavailable
}

// instantiate creates a fresh, anonymous instance (FN-010) with no file
// system, arguments or environment, standard output and error bound to the
// log budget, and the engine's clock and randomness (ADR-0062 WASI shim).
func (e *Engine) instantiate(ctx context.Context, st *callState) (api.Module, error) {
	cfg := wazero.NewModuleConfig().
		WithName("").
		WithStartFunctions().
		WithStdout(stream{budget: st.budget, level: LogInfo}).
		WithStderr(stream{budget: st.budget, level: LogError}).
		WithWalltime(func() (int64, int32) {
			now := e.clock()
			return now.Unix(), int32(now.Nanosecond()) //nolint:gosec // below 1e9
		}, 1).
		WithNanotime(func() int64 { return e.clock().UnixNano() }, 1).
		WithRandSource(e.random)
	mod, err := e.rt.InstantiateModule(ctx, st.module.compiled, cfg)
	if err != nil {
		return nil, err //nolint:wrapcheck // classified by the caller
	}
	return mod, nil
}

// initialise runs the module's constructors when it exports them.
func initialise(ctx context.Context, mod api.Module, m *Module) error {
	if !m.initialise {
		return nil
	}
	_, err := mod.ExportedFunction(exportInitialise).Call(ctx)
	return err //nolint:wrapcheck // classified by the caller
}

// abort ends the call from inside a host function: it records the fault,
// closes the instance with exitFault and unwinds the guest with the same
// panic wazero's own proc_exit uses. The engine recovers it and Call returns
// a *sys.ExitError; the panic never leaves wazero.
func (st *callState) abort(ctx context.Context, mod api.Module, err error) {
	st.fault = err
	_ = mod.CloseWithExitCode(ctx, exitFault)
	panic(sys.NewExitError(exitFault))
}

// fail aborts the call with PLX-7007 and a message.
func (st *callState) fail(ctx context.Context, mod api.Module, msg string) {
	st.abort(ctx, mod, plxerr.New(plxerr.FunctionTrapped, "%s", msg))
}

// propagate ends the call with the error of a nested call into the guest.
func (st *callState) propagate(ctx context.Context, mod api.Module, err error) {
	var exit *sys.ExitError
	if errors.As(err, &exit) {
		panic(exit)
	}
	st.abort(ctx, mod, plxerr.Wrap(plxerr.FunctionTrapped, err, "the guest trapped: %s", firstLine(err)))
}

// read copies n bytes at ptr out of the guest's memory, bounds-checked; a
// bad range ends the call with PLX-7007.
func (st *callState) read(ctx context.Context, mod api.Module, ptr, n uint64) []byte {
	view, ok := mod.Memory().Read(api.DecodeU32(ptr), api.DecodeU32(n))
	if !ok {
		st.fail(ctx, mod, "a host call received a pointer outside the guest's memory")
		return nil
	}
	return bytes.Clone(view)
}

// putBuffer hands data to the guest: it allocates through plux_alloc, writes
// into the allocation and returns the packed (ptr << 32) | len, or a negative
// status if the guest could not allocate.
func (st *callState) putBuffer(ctx context.Context, mod api.Module, data []byte) int64 {
	if len(data) == 0 {
		return 0
	}
	if len(data) > math.MaxInt32 {
		return statusLimit
	}
	res, err := mod.ExportedFunction("plux_alloc").Call(ctx, uint64(len(data)))
	if err != nil {
		st.propagate(ctx, mod, err)
		return 0
	}
	ptr := api.DecodeU32(res[0])
	if ptr == 0 {
		return statusLimit
	}
	if ptr >= 1<<31 || !mod.Memory().Write(ptr, data) {
		st.fail(ctx, mod, "plux_alloc returned a buffer outside the guest's memory")
		return 0
	}
	return int64(ptr)<<32 | int64(len(data))
}

// classify maps the error of a guest call to the registered error: the
// deadline and a cancellation first, then a refused memory growth (FN-011),
// then a fault a host function recorded, and otherwise a trap.
func classify(st *callState, err error) error {
	var exit *sys.ExitError
	if errors.As(err, &exit) {
		switch exit.ExitCode() {
		case sys.ExitCodeDeadlineExceeded:
			return plxerr.New(plxerr.FunctionTimeout, "the function exceeded its time limit")
		case sys.ExitCodeContextCanceled:
			return plxerr.New(plxerr.FunctionCancelled, "the invocation was cancelled")
		}
	}
	switch {
	case st.memory.refused:
		return plxerr.New(plxerr.FunctionMemoryExceeded, "the function exceeded its memory limit of %d bytes", st.memory.limit)
	case st.fault != nil:
		return st.fault
	case st.denied:
		return plxerr.Wrap(plxerr.FunctionCapabilityDenied, err, "the function stopped after a capability it was not granted was refused: %s", firstLine(err))
	default:
		return plxerr.Wrap(plxerr.FunctionTrapped, err, "the function trapped: %s", firstLine(err))
	}
}

// firstLine is the first line of an error, which for a wazero trap leaves out
// the stack trace.
func firstLine(err error) string {
	line, _, _ := strings.Cut(err.Error(), "\n")
	return line
}
