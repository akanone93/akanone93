// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/tetratelabs/wazero/api"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Schemas are the JSON Schemas of one function, as its module reports them
// through plux_schemas (FN-004).
type Schemas struct {
	// Input is the JSON Schema of the function's input.
	Input json.RawMessage
	// Output is the JSON Schema of the function's output.
	Output json.RawMessage
}

// schemasKeys are the two members of the document plux_schemas returns.
const (
	schemasInputKey  = "input"
	schemasOutputKey = "output"
)

// ReadSchemas compiles a module built with the feature plux-schema and reads
// the schemas of its first count functions through plux_schemas (FN-004, ADR-0063).
// Each function is read in a fresh instance with no capability granted
// (FN-012), under the time, memory and output limits of lim; the other limits
// of lim do not apply. A module that does not export plux_schemas, a trap, a
// limit, a result that is not `{"input": …, "output": …}` with two JSON objects,
// or a count that is not positive is PLX-7204.
func (e *Engine) ReadSchemas(ctx context.Context, wasm []byte, count int, lim Limits) ([]Schemas, error) {
	if count < 1 {
		return nil, plxerr.New(plxerr.FunctionSchemaDerivationFailed, "a module has at least one function; asked for %d", count)
	}
	m, err := e.Compile(ctx, wasm, PlacementServer)
	if err != nil {
		return nil, plxerr.Wrap(plxerr.FunctionSchemaDerivationFailed, err, "the schema module is not a function module")
	}
	defer func() { _ = m.Close(context.WithoutCancel(ctx)) }()
	if !m.exportsSchemas {
		return nil, plxerr.New(plxerr.FunctionSchemaDerivationFailed, "the module does not export plux_schemas; build it with the feature plux-schema")
	}
	out := make([]Schemas, count)
	for i := range out {
		if out[i], err = e.schemasOf(ctx, m, i, lim); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// schemasOf reads the schemas of the function at index in a fresh instance.
func (e *Engine) schemasOf(ctx context.Context, m *Module, index int, lim Limits) (Schemas, error) {
	c := &Call{Index: uint32(index), Limits: lim, Host: unavailableHost{}} //nolint:gosec // G115: index < count, a small positive int.
	if err := c.validate(); err != nil {
		return Schemas{}, plxerr.Wrap(plxerr.FunctionSchemaDerivationFailed, err, "the schema limits are invalid")
	}
	if uint64(m.minPages)*pageBytes > lim.MemoryBytes {
		return Schemas{}, plxerr.New(plxerr.FunctionSchemaDerivationFailed,
			"the schema module needs %d bytes at start; the limit is %d", uint64(m.minPages)*pageBytes, lim.MemoryBytes)
	}
	st := &callState{
		engine: e, module: m, call: c,
		memory:  &cappedMemory{limit: lim.MemoryBytes},
		budget:  &logBudget{limit: lim.LogBytes},
		ctxJSON: contextJSON(ContextInfo{}, m.placement),
	}
	ctx, cancel := context.WithTimeout(ctx, lim.Timeout)
	defer cancel()
	ctx = st.attach(ctx)
	mod, err := e.instantiate(ctx, st)
	if err != nil {
		return Schemas{}, plxerr.Wrap(plxerr.FunctionSchemaDerivationFailed, classify(st, err), "function %d", index)
	}
	defer func() { _ = mod.Close(context.WithoutCancel(ctx)) }()
	if err := initialise(ctx, mod, m); err != nil {
		return Schemas{}, plxerr.Wrap(plxerr.FunctionSchemaDerivationFailed, classify(st, err), "function %d", index)
	}
	res, err := mod.ExportedFunction("plux_schemas").Call(ctx, uint64(index)) //nolint:gosec // G115: see above.
	if err != nil {
		return Schemas{}, plxerr.Wrap(plxerr.FunctionSchemaDerivationFailed, classify(st, err), "function %d", index)
	}
	doc, err := readPacked(mod, res[0], lim.OutputBytes)
	if err != nil {
		return Schemas{}, plxerr.Wrap(plxerr.FunctionSchemaDerivationFailed, err, "function %d", index)
	}
	s, err := parseSchemas(doc)
	if err != nil {
		return Schemas{}, plxerr.Wrap(plxerr.FunctionSchemaDerivationFailed, err, "function %d", index)
	}
	return s, nil
}

// readPacked copies the `(ptr << 32) | len` buffer plux_schemas returned out
// of the instance's memory, refusing one outside the memory or above limit.
func readPacked(mod api.Module, packed uint64, limit int) ([]byte, error) {
	ptr, n := api.DecodeU32(packed>>32), api.DecodeU32(packed)
	if n == 0 || int64(n) > int64(limit) {
		return nil, fmt.Errorf("the schemas document is empty or above %d bytes", limit)
	}
	view, ok := mod.Memory().Read(ptr, n)
	if !ok {
		return nil, fmt.Errorf("the schemas document lies outside the memory")
	}
	return bytes.Clone(view), nil
}

// parseSchemas checks the document of plux_schemas: a JSON object with
// exactly the members input and output, each a JSON object.
func parseSchemas(doc []byte) (Schemas, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(doc, &members); err != nil {
		return Schemas{}, fmt.Errorf("the schemas document is not a JSON object: %w", err)
	}
	if len(members) != 2 {
		return Schemas{}, fmt.Errorf("the schemas document must have exactly the members %q and %q", schemasInputKey, schemasOutputKey)
	}
	var s Schemas
	for key, dst := range map[string]*json.RawMessage{schemasInputKey: &s.Input, schemasOutputKey: &s.Output} {
		raw, ok := members[key]
		if !ok {
			return Schemas{}, fmt.Errorf("the schemas document has no %q member", key)
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
			return Schemas{}, fmt.Errorf("the %s schema is not a JSON object", key)
		}
		*dst = raw
	}
	return s, nil
}
