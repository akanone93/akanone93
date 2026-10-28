// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package renderworker

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/renderpb"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// CatalogueAllowance is the room a request has beyond its two projects for
// the function catalogue: the declarations of at most app.functions functions
// with their aliases and the schemas derived from their types. It is a bound
// on the transport, not a limit of the registry.
const CatalogueAllowance = 8 << 20

// ErrCatalogue is returned for a function catalogue that cannot be sent or
// read: more functions than app.functions allows, or a declaration that is
// not valid.
var ErrCatalogue = errors.New("renderworker: the function catalogue is not valid")

// maxFunctions is the number of distinct function names a catalogue may name:
// the registry's maximum of app.functions (LIM-001).
func maxFunctions() int {
	def, _ := limits.Lookup(limits.AppFunctions)
	return int(def.Max)
}

// EncodeCatalogue lists the answers of a function catalogue for a render
// request. A catalogue that cannot list its answers is sent as empty. It is
// refused when it names more functions than app.functions allows or is larger
// than CatalogueAllowance.
func EncodeCatalogue(cat compiler.FunctionCatalogue) ([]*renderpb.FunctionDeclaration, error) {
	lister, ok := cat.(compiler.FunctionLister)
	if !ok {
		return nil, nil
	}
	entries := lister.FunctionEntries()
	out := make([]*renderpb.FunctionDeclaration, 0, len(entries))
	names := map[string]struct{}{}
	size := 0
	for _, e := range entries {
		names[e.Name] = struct{}{}
		if len(names) > maxFunctions() {
			return nil, fmt.Errorf("%w: more than %d functions", ErrCatalogue, maxFunctions())
		}
		d, err := encodeEntry(e)
		if err != nil {
			return nil, err
		}
		if size += declarationSize(d); size > CatalogueAllowance {
			return nil, fmt.Errorf("%w: more than %d bytes", ErrCatalogue, CatalogueAllowance)
		}
		out = append(out, d)
	}
	return out, nil
}

func encodeEntry(e compiler.FunctionEntry) (*renderpb.FunctionDeclaration, error) {
	d := &renderpb.FunctionDeclaration{
		Name: e.Name, Alias: e.Alias, Placement: string(e.Info.Placement), Version: e.Info.Version,
		InputSchema: e.Info.InputSchema, OutputSchema: e.Info.OutputSchema,
		RequiresAttestation: e.Info.RequiresAttestation, RequiresSca: e.Info.RequiresSCA,
	}
	var err error
	if e.Info.Capabilities != nil {
		if d.CapabilitiesJson, err = json.Marshal(e.Info.Capabilities); err != nil {
			return nil, fmt.Errorf("%w: capabilities of %s: %w", ErrCatalogue, e.Name, err)
		}
	}
	if e.Info.Limits != nil {
		if d.LimitsJson, err = json.Marshal(e.Info.Limits); err != nil {
			return nil, fmt.Errorf("%w: limits of %s: %w", ErrCatalogue, e.Name, err)
		}
	}
	return d, nil
}

func declarationSize(d *renderpb.FunctionDeclaration) int {
	return len(d.GetName()) + len(d.GetAlias()) + len(d.GetPlacement()) + len(d.GetCapabilitiesJson()) +
		len(d.GetLimitsJson()) + len(d.GetInputSchema()) + len(d.GetOutputSchema())
}

// DecodeCatalogue builds the catalogue a render compiles against from the
// declarations of a request. The declarations come from the server but are
// checked as untrusted input: the number of names, the placement and the JSON
// of each are validated before the compiler sees them.
func DecodeCatalogue(decls []*renderpb.FunctionDeclaration, deviceFunctions bool) (compiler.FunctionCatalogue, error) {
	names := map[string]struct{}{}
	size := 0
	entries := make([]compiler.FunctionEntry, 0, len(decls))
	for _, d := range decls {
		if d.GetName() == "" {
			return nil, fmt.Errorf("%w: a declaration has no name", ErrCatalogue)
		}
		names[d.GetName()] = struct{}{}
		if len(names) > maxFunctions() {
			return nil, fmt.Errorf("%w: more than %d functions", ErrCatalogue, maxFunctions())
		}
		if size += declarationSize(d); size > CatalogueAllowance {
			return nil, fmt.Errorf("%w: more than %d bytes", ErrCatalogue, CatalogueAllowance)
		}
		info, err := decodeInfo(d)
		if err != nil {
			return nil, err
		}
		entries = append(entries, compiler.FunctionEntry{Name: d.GetName(), Alias: d.GetAlias(), Info: info})
	}
	return compiler.NewFunctionSet(entries, deviceFunctions), nil
}

func decodeInfo(d *renderpb.FunctionDeclaration) (compiler.FunctionInfo, error) {
	placement := schema.Placement(d.GetPlacement())
	if !placement.Valid() {
		return compiler.FunctionInfo{}, fmt.Errorf("%w: %s has the placement %q", ErrCatalogue, d.GetName(), d.GetPlacement())
	}
	info := compiler.FunctionInfo{
		Placement: placement, Version: d.GetVersion(),
		InputSchema: d.GetInputSchema(), OutputSchema: d.GetOutputSchema(),
		RequiresAttestation: d.GetRequiresAttestation(), RequiresSCA: d.GetRequiresSca(),
	}
	for _, raw := range [][]byte{info.InputSchema, info.OutputSchema} {
		if len(raw) > 0 && !json.Valid(raw) {
			return compiler.FunctionInfo{}, fmt.Errorf("%w: a schema of %s is not JSON", ErrCatalogue, d.GetName())
		}
	}
	if len(d.GetCapabilitiesJson()) > 0 {
		info.Capabilities = new(schema.FunctionCapabilities)
		if err := json.Unmarshal(d.GetCapabilitiesJson(), info.Capabilities); err != nil {
			return compiler.FunctionInfo{}, fmt.Errorf("%w: capabilities of %s: %w", ErrCatalogue, d.GetName(), err)
		}
	}
	if len(d.GetLimitsJson()) > 0 {
		info.Limits = new(schema.FunctionLimits)
		if err := json.Unmarshal(d.GetLimitsJson(), info.Limits); err != nil {
			return compiler.FunctionInfo{}, fmt.Errorf("%w: limits of %s: %w", ErrCatalogue, d.GetName(), err)
		}
	}
	return info, nil
}

// deviceStubs stands in for the builder's linked device modules: a render
// draws pages and never calls a function, so the module a plugin's
// device-placed functions would link to carries no code, only the names the
// compiler checks the bundle against. The bundle is never installed (FN-051).
type deviceStubs struct{}

// deviceABIVersion is the host ABI version the stub claims (FN-015).
const deviceABIVersion = 1

// Module implements compiler.DeviceModules.
func (deviceStubs) Module(_ string, fns []compiler.DeviceFunctionRef) (compiler.DeviceModule, error) {
	index := make(map[string]uint32, len(fns))
	for i, f := range fns {
		index[f.Name] = uint32(i) //nolint:gosec // G115: bounded by the functions of one plugin.
	}
	return compiler.DeviceModule{ABIVersion: deviceABIVersion, Index: index}, nil
}
