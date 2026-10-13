// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"fmt"

	"github.com/nightCode42/plux3/backend/internal/fnhost"
)

// WasmSchemaReader is the [SchemaReader] over the function host: it runs the
// schema build in wazero, in a fresh instance per function with no capability
// granted, so user code never runs natively in the builder (FN-004, ADR-0063).
// It is safe for concurrent use when its Engine is.
type WasmSchemaReader struct {
	engine *fnhost.Engine
	limits fnhost.Limits
}

// NewWasmSchemaReader returns a reader over the engine. Only the timeout, the
// memory and the output limit of lim apply; the caller owns the engine.
func NewWasmSchemaReader(engine *fnhost.Engine, lim fnhost.Limits) *WasmSchemaReader {
	return &WasmSchemaReader{engine: engine, limits: lim}
}

// Schemas implements [SchemaReader].
func (r *WasmSchemaReader) Schemas(ctx context.Context, module []byte, count int) ([]FunctionSchemas, error) {
	got, err := r.engine.ReadSchemas(ctx, module, count, r.limits)
	if err != nil {
		return nil, fmt.Errorf("builder.WasmSchemaReader: %w", err)
	}
	out := make([]FunctionSchemas, len(got))
	for i, g := range got {
		out[i] = FunctionSchemas{Input: g.Input, Output: g.Output}
	}
	return out, nil
}
