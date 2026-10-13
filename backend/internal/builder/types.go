// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"io/fs"
)

// ABIVersion is the version of the function host ABI the builder produces
// modules for (ADR-0062, plan P7 E6).
const ABIVersion = 1

// Placement says where a function runs; it fixes the contexts its handler can
// use (FN-008) and is asserted against the crate at compile time (PLX-7205).
type Placement string

// The placements of a function document.
const (
	// PlacementDevice runs on the device, in the WAMR interpreter.
	PlacementDevice Placement = "device"
	// PlacementServer runs on the server, in the `fnrunner` role.
	PlacementServer Placement = "server"
)

// Valid reports whether p is a known placement.
func (p Placement) Valid() bool {
	return p == PlacementDevice || p == PlacementServer
}

// FunctionSource is one function to build: the identity its document gives it
// and the Rust crate that implements it.
type FunctionSource struct {
	// Name is the name in the function document; it orders the functions of a
	// module and so fixes each function's index.
	Name string
	// Placement is the placement in the function document.
	Placement Placement
	// Files is the root of the crate: Cargo.toml, Cargo.lock (FN-003), src/
	// and the output of `cargo vendor` in vendor/ (FN-003). Entries other than
	// directories and regular files are refused.
	Files fs.FS
}

// FunctionSchemas are the JSON Schemas derived from a function's Rust types
// (FN-004).
type FunctionSchemas struct {
	// Input is the JSON Schema of the function's input.
	Input []byte
	// Output is the JSON Schema of the function's output.
	Output []byte
}

// SchemaReader reads the schemas out of a module built with the feature
// plux-schema (FN-004). The implementation runs the module in the WebAssembly
// host; the builder never executes user code natively for this.
type SchemaReader interface {
	// Schemas returns the schemas of the count functions of module, in index
	// order.
	Schemas(ctx context.Context, module []byte, count int) ([]FunctionSchemas, error)
}

// Options are the per-build switches.
type Options struct {
	// Verify builds the artefact twice into separate target directories and
	// fails with PLX-7206 unless the two modules are byte-identical (FN-019).
	Verify bool
}

// Function describes one function of a module.
type Function struct {
	// Name is the name in the function document.
	Name string
	// Index is the function's index in the module's dispatch table.
	Index int
	// Placement is the placement the crate was asserted against.
	Placement Placement
	// InputSchema is the canonical JSON (RFC 8785) of the input schema.
	InputSchema []byte
	// OutputSchema is the canonical JSON (RFC 8785) of the output schema.
	OutputSchema []byte
}

// Dependency is one registry crate of the build, from the lock file (FN-003,
// FN-005).
type Dependency struct {
	// Name is the crate name.
	Name string `json:"name"`
	// Version is the exact version.
	Version string `json:"version"`
	// Checksum is the SHA-256 of the crate archive, as the lock file records it.
	Checksum string `json:"checksum"`
}

// Provenance records how a module was built (FN-005).
type Provenance struct {
	// SourceSHA256 is the hash of the canonical source archive: the SHA-256
	// over the `path NUL hex(SHA-256) LF` line of every source file, vendor
	// files included, in bytewise path order. A path is the function's name, a
	// slash and the path inside the crate.
	SourceSHA256 string
	// SDKSHA256 is the hash of the pinned SDK crate copied into the build, over
	// the same construction as SourceSHA256.
	SDKSHA256 string
	// Dependencies are the registry crates of the lock file, sorted by name and
	// version.
	Dependencies []Dependency
	// Toolchain is the full `rustc --version` line.
	Toolchain string
	// WasmOpt is the optimiser version, "version_132".
	WasmOpt string
	// BuildTime is when the build ran, in UTC, from the injected clock.
	BuildTime string
	// BuilderIdentity names the builder instance that produced the module.
	BuilderIdentity string
}

// Artifact is the output of one build (FN-005).
type Artifact struct {
	// Module is the optimised WebAssembly module.
	Module []byte
	// ModuleSHA256 is the lower-case hexadecimal SHA-256 of Module.
	ModuleSHA256 string
	// ABIVersion is the ABI version the module exports.
	ABIVersion int
	// Functions lists the module's functions in index order.
	Functions []Function
	// Provenance records the inputs and tools of the build.
	Provenance Provenance
}
