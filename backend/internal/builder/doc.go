// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package builder turns the Rust sources of Plux Functions into a signed-ready
// WebAssembly module (spec §16, ADR-0063, plan P7 E5, E7, E10).
//
// The pipeline is the one the `builder` role runs on the server and
// `plux fn build` runs on a developer machine (FN-016):
//
//  1. Check the pinned toolchain, rustc 1.97.0 (FN-001), and the pinned
//     wasm-opt, Binaryen version 132 (E10).
//  2. Lay out a build workspace in a scratch directory: a copy of the pinned
//     SDK crate `plux-fn`, each function crate, a generated wrapper crate
//     `plux-module` that links them through `plux_fn::module!`, and one
//     vendored source directory merged from the sources' own `vendor/`
//     directories. `[patch.crates-io]` resolves the unpublished `plux-fn`
//     to the SDK copy.
//  3. Enforce the lock discipline (FN-003): resolution runs offline against the
//     vendored sources only and must reproduce every version and checksum the
//     functions' own `Cargo.lock` files recorded. Anything else is PLX-7201.
//  4. Enforce the allowlist of build scripts and procedural macros (FN-016,
//     PLX-7202) over the crates that are actually built.
//  5. Build once with the feature `plux-schema` and read the JSON Schemas out
//     of that module through a [SchemaReader] (FN-004); user code never runs
//     natively beyond allowlisted build scripts and procedural macros.
//  6. Build the artefact without the feature, optimise it with wasm-opt and,
//     with [Options.Verify], build it a second time into another target
//     directory and compare the bytes (FN-019, PLX-7206).
//
// Every child process runs through a [Sandbox] with a clean environment, a
// new network namespace and resource limits (FN-016, PLX-7203). Where the
// kernel refuses the namespaces, the Linux sandbox returns
// [ErrSandboxUnavailable]; the builder never falls back silently. [NoSandbox]
// exists for developer machines and for the `builder.sandbox: none`
// development setting, whose caller logs the distinct warning.
//
// The result is an [Artifact] with its provenance (FN-005). Its Statement is
// the canonical JSON that `signing/` signs; the builder holds no key and no
// database credential (L-4).
//
// Determinism (FN-019): the workspace layout, the manifests, the environment
// and the order of functions are fixed by the input; no wall clock or random
// value reaches the module. The build time is only recorded in the
// provenance, from an injected clock.
//
// Function crates must be self-contained manifests: they declare
// `plux-fn = "0.1"`, must not declare a `[workspace]` table and must not
// inherit values from a workspace. A [Builder] is safe for concurrent use;
// each Build call works in its own scratch directory.
package builder
