// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/api"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Invoke runs one call in a fresh instance of m (FN-010) under the call's
// limits and grants (FN-011, FN-012). The Stats are returned whether or not
// the call succeeded.
//
// A function's own failure is an Outcome with Business set. A system failure
// is a *plxerr.Error: PLX-7001 timeout, 7002 memory, 7004 output too large,
// 7007 trap or malformed result, 7008 and 7009 for a guest that refused its
// input or could not encode its output, 7021 cancellation and 7022 input too
// large. A call that is itself wrong (no timeout) is a plain error.
func (e *Engine) Invoke(ctx context.Context, m *Module, c Call) (Outcome, Stats, error) {
	start := e.clock()
	if c.Host == nil {
		c.Host = unavailableHost{}
	}
	st := &callState{
		engine: e, module: m, call: &c,
		memory: &cappedMemory{limit: c.Limits.MemoryBytes, pool: e.pool},
		budget: &logBudget{limit: c.Limits.LogBytes, sink: c.Log},
	}
	out, err := e.invoke(ctx, st)
	stats := Stats{
		Duration:        e.clock().Sub(start),
		PeakMemoryBytes: st.memory.peak,
		HTTPCalls:       st.httpCalls,
		LogBytes:        st.budget.used,
	}
	return out, stats, err
}

// invoke is Invoke without the bookkeeping.
func (e *Engine) invoke(ctx context.Context, st *callState) (Outcome, error) {
	c, m := st.call, st.module
	if err := c.validate(); err != nil {
		return Outcome{}, err
	}
	if len(c.Input) > c.Limits.InputBytes {
		return Outcome{}, plxerr.New(plxerr.FunctionInputTooLarge,
			"the input of %d bytes exceeds the limit of %d", len(c.Input), c.Limits.InputBytes)
	}
	if uint64(m.minPages)*pageBytes > c.Limits.MemoryBytes {
		return Outcome{}, plxerr.New(plxerr.FunctionMemoryExceeded,
			"the module needs %d bytes at start; the limit is %d", uint64(m.minPages)*pageBytes, c.Limits.MemoryBytes)
	}
	st.ctxJSON = contextJSON(c.Context, m.placement)
	ctx, cancel := context.WithTimeout(ctx, c.Limits.Timeout)
	defer cancel()
	ctx = st.attach(ctx)
	defer st.memory.release() // after the instance closed: deferred first
	mod, err := e.instantiate(ctx, st)
	if err != nil {
		return Outcome{}, classify(st, err)
	}
	defer func() { _ = mod.Close(context.WithoutCancel(ctx)) }()
	packed, err := run(ctx, st, mod)
	if err != nil {
		return Outcome{}, classify(st, err)
	}
	if err := expired(ctx); err != nil {
		return Outcome{}, err
	}
	return decodeResult(mod, packed, c.Limits.OutputBytes)
}

// expired reports a guest that returned after its deadline or cancellation,
// before the runtime closed it: a host call bounded by the invocation, such
// as an HTTP request, ends with the deadline and hands the guest an error it
// can still answer. The invocation exceeded its time limit all the same
// (FN-011).
func expired(ctx context.Context) error {
	switch err := ctx.Err(); {
	case errors.Is(err, context.DeadlineExceeded):
		return plxerr.New(plxerr.FunctionTimeout, "the function exceeded its time limit")
	case err != nil:
		return plxerr.New(plxerr.FunctionCancelled, "the invocation was cancelled")
	}
	return nil
}

// run initialises the instance, passes the input through plux_alloc and
// calls plux_invoke.
func run(ctx context.Context, st *callState, mod api.Module) (uint64, error) {
	c := st.call
	if err := initialise(ctx, mod, st.module); err != nil {
		return 0, err
	}
	var ptr uint32
	if len(c.Input) > 0 {
		res, err := mod.ExportedFunction("plux_alloc").Call(ctx, uint64(len(c.Input)))
		if err != nil {
			return 0, fmt.Errorf("plux_alloc: %w", err)
		}
		ptr = api.DecodeU32(res[0])
		if ptr == 0 || !mod.Memory().Write(ptr, c.Input) {
			st.fault = plxerr.New(plxerr.FunctionTrapped, "plux_alloc did not return room for the input")
			return 0, errors.New("plux_alloc returned an unusable buffer")
		}
	}
	res, err := mod.ExportedFunction("plux_invoke").Call(ctx,
		uint64(c.Index), uint64(ptr), uint64(len(c.Input)))
	if err != nil {
		return 0, fmt.Errorf("plux_invoke: %w", err)
	}
	return res[0], nil
}

// decodeResult reads the packed `tag || JSON` result (ADR-0062). A range
// outside the memory, an empty result or an unknown tag is PLX-7007; a
// payload over the output limit is PLX-7004 and is not copied.
func decodeResult(mod api.Module, packed uint64, outputLimit int) (Outcome, error) {
	ptr, n := uint32(packed>>32), uint32(packed) //nolint:gosec // G115: the two halves of the packed value.
	view, ok := mod.Memory().Read(ptr, n)
	if !ok || n == 0 {
		return Outcome{}, plxerr.New(plxerr.FunctionTrapped, "the result is empty or lies outside the memory")
	}
	tag, payload := view[0], view[1:]
	if len(payload) > outputLimit {
		return Outcome{}, plxerr.New(plxerr.FunctionOutputTooLarge,
			"the output of %d bytes exceeds the limit of %d", len(payload), outputLimit)
	}
	switch tag {
	case tagOutput:
		return Outcome{Output: bytes.Clone(payload)}, nil
	case tagBusinessError:
		return decodeBusiness(payload)
	case tagInputRefused:
		return Outcome{}, plxerr.New(plxerr.FunctionInputInvalid, "the function refused its input: %s", guestMessage(payload))
	case tagOutputFailed:
		return Outcome{}, plxerr.New(plxerr.FunctionOutputInvalid, "the function could not encode its output: %s", guestMessage(payload))
	default:
		return Outcome{}, plxerr.New(plxerr.FunctionTrapped, "the result has the unknown tag %d", tag)
	}
}

// decodeBusiness parses a tag-1 payload; a malformed one is PLX-7009.
func decodeBusiness(payload []byte) (Outcome, error) {
	var be BusinessError
	if err := json.Unmarshal(payload, &be); err != nil || be.Code == "" || be.MessageKey == "" {
		return Outcome{}, plxerr.New(plxerr.FunctionOutputInvalid, "the business error lacks a code or a message key")
	}
	if len(be.Params) > 0 && be.Params[0] != '{' {
		return Outcome{}, plxerr.New(plxerr.FunctionOutputInvalid, "the business error params are not an object")
	}
	be.Params = bytes.Clone(be.Params)
	return Outcome{Business: &be}, nil
}

// guestMessage extracts `message` from a tag-2 or tag-3 payload.
func guestMessage(payload []byte) string {
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(payload, &m) != nil || m.Message == "" {
		return "no message"
	}
	return m.Message
}

// contextJSON renders what context() returns: app, environment, placement
// and assurance level, in that order.
func contextJSON(info ContextInfo, p Placement) []byte {
	out, _ := json.Marshal(struct { //nolint:errchkjson // strings only
		App            string    `json:"app"`
		Environment    string    `json:"environment"`
		Placement      Placement `json:"placement"`
		AssuranceLevel string    `json:"assuranceLevel"`
	}{info.App, info.Environment, p, info.AssuranceLevel})
	return out
}
