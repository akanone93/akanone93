// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// Files `plux fn build` writes beside the module, which the offline
// catalogue reads.
const (
	inputSchemaFile  = "input.schema.json"
	outputSchemaFile = "output.schema.json"
)

// localVersion is the version of every function offline: the one local
// build. Versions are a server concept (FN-006), so a bundle built offline
// pins this one.
const localVersion = 1

// localCatalogue is the function catalogue of an offline compilation
// (CLI-005): the function documents of the project's functions/<name>/ and
// the schemas `plux fn build` derived from their Rust types (FN-004). It
// has no deployed versions, so every alias resolves to the one local build,
// and no security profile, so device placement is allowed; the server
// judges both when the project is published (FN-055). It is read once and
// never changed, so it is safe for concurrent use.
type localCatalogue struct {
	infos map[string]compiler.FunctionInfo
}

var _ compiler.FunctionCatalogue = (*localCatalogue)(nil)

// Function implements compiler.FunctionCatalogue.
func (c *localCatalogue) Function(name, _ string) (compiler.FunctionInfo, bool) {
	info, ok := c.infos[name]
	return info, ok
}

// DeviceFunctionsEnabled implements compiler.FunctionCatalogue.
func (*localCatalogue) DeviceFunctionsEnabled() bool { return true }

// builtDir is where `plux fn build` wrote the output of a function.
func builtDir(project, name string) string {
	return filepath.Join(crateDir(project, name), filepath.FromSlash(outputDir))
}

// missingBuild is the error of a function whose build output is absent.
func missingBuild(name string) error {
	return fmt.Errorf("function %s has no build output: run plux fn build %s", name, name)
}

// loadLocalCatalogue reads the functions of a project directory. A function
// with a document but no build output makes the project unusable offline,
// and every such function is named. A document that cannot be read is left
// out for the compiler to report.
func loadLocalCatalogue(project string) (*localCatalogue, error) {
	cat := &localCatalogue{infos: map[string]compiler.FunctionInfo{}}
	entries, err := os.ReadDir(filepath.Join(project, "functions"))
	if errors.Is(err, fs.ErrNotExist) {
		return cat, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the functions of %s: %w", project, err)
	}
	var missing []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, name, ok := readLocalFunction(project, entry.Name())
		if !ok {
			continue
		}
		if info.InputSchema == nil {
			missing = append(missing, missingBuild(name))
			continue
		}
		cat.infos[name] = info
	}
	if len(missing) > 0 {
		return nil, errors.Join(missing...)
	}
	return cat, nil
}

// readLocalFunction describes the function in functions/<dir>/. ok is false
// when the directory holds no readable function document. The schemas are
// nil when the function was not built.
func readLocalFunction(project, dir string) (info compiler.FunctionInfo, name string, ok bool) {
	data, err := os.ReadFile(filepath.Join(project, "functions", dir, builder.DeclarationFile)) //nolint:gosec // G304: the developer's own project.
	if err != nil {
		return info, "", false
	}
	var doc struct {
		schema.FunctionDocument
		Attestation bool `json:"attestation"`
		SCA         bool `json:"sca"`
	}
	if json.Unmarshal(data, &doc) != nil || doc.Name == "" {
		return info, "", false
	}
	info = compiler.FunctionInfo{
		Placement: doc.Placement, Capabilities: doc.Capabilities, Limits: doc.Limits, Version: localVersion,
		RequiresAttestation: doc.Attestation, RequiresSCA: doc.SCA,
	}
	built := builtDir(project, dir)
	in, inErr := os.ReadFile(filepath.Join(built, inputSchemaFile))    //nolint:gosec // G304: the developer's own build output.
	out, outErr := os.ReadFile(filepath.Join(built, outputSchemaFile)) //nolint:gosec // G304: the developer's own build output.
	if inErr != nil || outErr != nil {
		return info, doc.Name, true
	}
	info.InputSchema, info.OutputSchema = in, out
	return info, doc.Name, true
}

// unlinkedModules stands in for the device modules where the bundles are
// not kept (validate, codegen and the checks of other commands): the
// compiler needs a module source to finish a plugin that grants a device
// function, but nothing is written, so an empty module is enough.
type unlinkedModules struct{}

// emptyModule is the smallest valid WebAssembly module: the header alone.
var emptyModule = []byte{0x00, 'a', 's', 'm', 0x01, 0x00, 0x00, 0x00}

// Module implements compiler.DeviceModules.
func (unlinkedModules) Module(_ string, refs []compiler.DeviceFunctionRef) (compiler.DeviceModule, error) {
	index := make(map[string]uint32, len(refs))
	for i, r := range refs {
		index[r.Name] = uint32(i) //nolint:gosec // G115: a plugin grants few functions.
	}
	return compiler.DeviceModule{Wasm: slices.Clone(emptyModule), ABIVersion: builder.ABIVersion, Index: index}, nil
}

// localModules links the device functions of a plugin from the local build
// output, for plux build.
type localModules struct {
	e       env
	ctx     context.Context //nolint:containedctx // the compiler's interface has no context to pass.
	project string
}

var _ compiler.DeviceModules = localModules{}

// Module implements compiler.DeviceModules (FN-051). One function is its
// own module, which `plux fn build` wrote. Several are linked from their
// crates by the local toolchain, as `plux fn build` builds one, so that
// needs the Rust toolchain, the SDK and the crates in functions/<name>/.
func (m localModules) Module(pluginKey string, refs []compiler.DeviceFunctionRef) (compiler.DeviceModule, error) {
	if len(refs) == 1 {
		name := refs[0].Name
		wasm, err := os.ReadFile(filepath.Join(builtDir(m.project, name), moduleName)) //nolint:gosec // G304: the developer's own build output.
		if err != nil {
			return compiler.DeviceModule{}, missingBuild(name)
		}
		return compiler.DeviceModule{Wasm: wasm, ABIVersion: builder.ABIVersion, Index: map[string]uint32{name: 0}}, nil
	}
	names := make([]string, len(refs))
	for i, r := range refs {
		names[i] = r.Name
	}
	mod, err := m.e.linkFunctions(m.ctx, m.project, names)
	if err != nil {
		return compiler.DeviceModule{}, fmt.Errorf("link %s for plugin %s: %w", strings.Join(names, ", "), pluginKey, err)
	}
	return mod, nil
}

// linkFunctions links the crates of several device functions into one
// module with the local toolchain, the way the builder role links them on the
// server (FN-051). names are sorted.
func (e env) linkFunctions(ctx context.Context, project string, names []string) (compiler.DeviceModule, error) {
	set := limits.Defaults()
	b, closeBuilder, err := e.localBuilder(ctx, "", set)
	if err != nil {
		return compiler.DeviceModule{}, err
	}
	defer closeBuilder()
	sources := make([]builder.FunctionSource, 0, len(names))
	for _, name := range names {
		files, decl, err := e.readCrate(ctx, project, name, set)
		if err != nil {
			return compiler.DeviceModule{}, err
		}
		sources = append(sources, decl.Source(files))
	}
	artifact, err := b.Build(ctx, sources, builder.Options{})
	if err != nil {
		return compiler.DeviceModule{}, fmt.Errorf("build: %w", err)
	}
	index := make(map[string]uint32, len(artifact.Functions))
	for _, f := range artifact.Functions {
		index[f.Name] = uint32(f.Index) //nolint:gosec // G115: an index is small and non-negative.
	}
	return compiler.DeviceModule{Wasm: artifact.Module, ABIVersion: uint32(artifact.ABIVersion), Index: index}, nil //nolint:gosec // G115: the ABI version is small and positive.
}
