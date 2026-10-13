// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// signature is a function type.
type signature struct {
	params  []api.ValueType
	results []api.ValueType
}

func (s signature) equal(params, results []api.ValueType) bool {
	return slices.Equal(s.params, params) && slices.Equal(s.results, results)
}

// Module is a validated, compiled function module. It is immutable and safe
// for concurrent use; Invoke creates a fresh instance from it for each call.
type Module struct {
	compiled   wazero.CompiledModule
	sha256     [sha256.Size]byte
	placement  Placement
	initialise bool
	// exportsSchemas is true for a module built with the feature plux-schema.
	exportsSchemas bool
	minPages       uint32
}

// SHA256 returns the digest of the module's bytes, the identity a release
// pins (FN-005).
func (m *Module) SHA256() [sha256.Size]byte { return m.sha256 }

// Placement returns the placement the module was compiled for.
func (m *Module) Placement() Placement { return m.placement }

// Close releases the compiled code.
func (m *Module) Close(ctx context.Context) error {
	if err := m.compiled.Close(ctx); err != nil {
		return fmt.Errorf("fnhost.Module.Close: %w", err)
	}
	return nil
}

// Compile validates a module for the placement and compiles it, before any
// guest code runs except plux_abi_version in a throwaway instance. It
// returns PLX-7018 for a module that is not a valid function module (missing
// or mistyped exports, an import outside plux_v1 and WASI, a start function,
// a server-only import on a device) and PLX-7017 for an unsupported ABI
// version.
func (e *Engine) Compile(ctx context.Context, wasm []byte, placement Placement) (*Module, error) {
	if placement != PlacementServer && placement != PlacementDevice {
		return nil, fmt.Errorf("fnhost.Compile: unknown placement %q", placement)
	}
	compiled, err := e.rt.CompileModule(ctx, wasm)
	if err != nil {
		return nil, plxerr.Wrap(plxerr.FunctionModuleInvalid, err, "the module does not compile")
	}
	m := &Module{compiled: compiled, sha256: sha256.Sum256(wasm), placement: placement}
	if err := e.validate(m, wasm); err != nil {
		_ = compiled.Close(ctx)
		return nil, err
	}
	if err := e.checkVersion(ctx, m); err != nil {
		_ = compiled.Close(ctx)
		return nil, err
	}
	return m, nil
}

// validate checks the structure of the module (FN-015, FN-052).
func (e *Engine) validate(m *Module, wasm []byte) error {
	if err := checkExports(m); err != nil {
		return err
	}
	if err := e.checkImports(m); err != nil {
		return err
	}
	if hasStartSection(wasm) {
		return plxerr.New(plxerr.FunctionModuleInvalid, "the module has a start function")
	}
	mem := m.compiled.ExportedMemories()["memory"]
	m.minPages = mem.Min()
	if uint64(m.minPages)*pageBytes > e.maxMem {
		return plxerr.New(plxerr.FunctionModuleInvalid,
			"the module's initial memory of %d pages exceeds the engine's maximum", m.minPages)
	}
	return nil
}

// checkExports verifies memory and the ABI functions with their types.
func checkExports(m *Module) error {
	if len(m.compiled.ImportedMemories()) > 0 {
		return plxerr.New(plxerr.FunctionModuleInvalid, "the module imports its memory")
	}
	if _, ok := m.compiled.ExportedMemories()["memory"]; !ok {
		return plxerr.New(plxerr.FunctionModuleInvalid, "the module does not export memory")
	}
	exports := m.compiled.ExportedFunctions()
	for _, spec := range requiredExports() {
		def, ok := exports[spec.name]
		if !ok {
			if spec.optional {
				continue
			}
			return plxerr.New(plxerr.FunctionModuleInvalid, "the module does not export %s", spec.name)
		}
		if !(signature{params: def.ParamTypes(), results: def.ResultTypes()}).equal(spec.params, spec.results) {
			return plxerr.New(plxerr.FunctionModuleInvalid, "the export %s has the wrong signature", spec.name)
		}
		switch spec.name {
		case exportInitialise:
			m.initialise = true
		case exportSchemas:
			m.exportsSchemas = true
		}
	}
	return nil
}

// checkImports allows only the known imports of plux_v1 and of WASI, with
// their signatures, and no server-only import on a device.
func (e *Engine) checkImports(m *Module) error {
	known := hostImports()
	for _, def := range m.compiled.ImportedFunctions() {
		module, name, _ := def.Import()
		sig := signature{params: def.ParamTypes(), results: def.ResultTypes()}
		switch module {
		case hostModule:
			spec, ok := known[name]
			if !ok {
				return plxerr.New(plxerr.FunctionModuleInvalid, "the module imports unknown function %s.%s", module, name)
			}
			if spec.serverOnly && m.placement == PlacementDevice {
				return plxerr.New(plxerr.FunctionModuleInvalid,
					"a device module cannot import the server-only function %s.%s", module, name)
			}
			if !sig.equal(spec.params, spec.results) {
				return plxerr.New(plxerr.FunctionModuleInvalid, "the import %s.%s has the wrong signature", module, name)
			}
		case wasiModule:
			want, ok := e.wasi[name]
			if !ok || !sig.equal(want.params, want.results) {
				return plxerr.New(plxerr.FunctionModuleInvalid, "the module imports unsupported WASI function %s", name)
			}
		default:
			return plxerr.New(plxerr.FunctionModuleInvalid, "the module imports from %q, which is not provided", module)
		}
	}
	return nil
}

// hasStartSection reports whether a validated module has a start section
// (id 8), which would run guest code at instantiation. A malformed layout
// reports false: the module was already validated by the compiler.
func hasStartSection(wasm []byte) bool {
	const header = 8
	pos := header
	for pos < len(wasm) {
		id := wasm[pos]
		pos++
		size, n := leb128(wasm[pos:])
		if n == 0 {
			return false
		}
		if id == 8 {
			return true
		}
		pos += n + int(size)
	}
	return false
}

// leb128 decodes an unsigned LEB128 value of at most 32 bits; n is zero when
// the encoding is malformed.
func leb128(b []byte) (value uint32, n int) {
	var shift uint
	for i, c := range b {
		if i >= 5 {
			return 0, 0
		}
		value |= uint32(c&0x7f) << shift
		if c&0x80 == 0 {
			return value, i + 1
		}
		shift += 7
	}
	return 0, 0
}

// checkVersion calls plux_abi_version in a throwaway instance with no
// capabilities (FN-015).
func (e *Engine) checkVersion(ctx context.Context, m *Module) error {
	st := &callState{
		engine: e, module: m,
		call:   &Call{Host: unavailableHost{}},
		memory: &cappedMemory{limit: e.maxMem},
		budget: &logBudget{},
	}
	ctx = st.attach(ctx)
	mod, err := e.instantiate(ctx, st)
	if err != nil {
		return plxerr.Wrap(plxerr.FunctionModuleInvalid, err, "the module does not instantiate")
	}
	defer func() { _ = mod.Close(context.WithoutCancel(ctx)) }()
	if err := initialise(ctx, mod, m); err != nil {
		return versionError(ctx, st, err)
	}
	res, err := mod.ExportedFunction("plux_abi_version").Call(ctx)
	if err != nil {
		return versionError(ctx, st, err)
	}
	if got := api.DecodeI32(res[0]); got != abiVersion {
		return plxerr.New(plxerr.FunctionABIUnsupported,
			"the module implements ABI version %d; this host supports %d", got, abiVersion)
	}
	return nil
}

// versionError classifies a failure while probing the module: the caller's
// deadline or cancellation keeps its own code, anything else means the module
// is not a function module.
func versionError(ctx context.Context, st *callState, err error) error {
	if ctx.Err() != nil {
		return classify(st, err)
	}
	return plxerr.Wrap(plxerr.FunctionModuleInvalid, err, "plux_abi_version failed: %s", firstLine(err))
}
