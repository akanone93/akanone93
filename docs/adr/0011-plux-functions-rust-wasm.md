# 0011. Plux Functions: Rust compiled to WebAssembly; explicit placement

- **Status:** Accepted (maintainer, 2026-10-13, P7 plan decisions D1, D2, D9)
- **Date:** 2026-10-13
- **Requirements:** `FN-001`–`FN-004`, `FN-010`, `FN-015`, `FN-040`, `FN-050`, `FN-056`

## Context and problem

Logic that declarative actions cannot express (a loan schedule, a fee calculation, an
eligibility rule) runs as a Plux Function: one module of WebAssembly that runs on the server
under `wazero` or, for pure calculations, on the device in an interpreter. The specification
fixes the shape (§16): one language-neutral artifact, explicit `server` or `device`
placement, fresh state per call, and strict budgets. This ADR answers three questions that
the specification left open: **which source language**, **which engine on the device**
(`FN-050` names three candidates), and **how the two placements share one artifact**.

An earlier draft chose standard Go. The P7 plan measured it against Rust on one function
(a 360-month loan schedule, JSON in and out) and the result changed the decision.

## Decision drivers

- Device budgets: a typical calculation in ≤ 20 ms p95 on the mid-tier device, a module of
  ≤ 4 MiB per plugin (`FN-056`), 32 MiB of memory (`FN-053`).
- Server budgets: first call after deploy in ≤ 50 ms, warm overhead ≤ 1 ms p99, a fresh
  instance per call (`FN-010`, `FN-023`).
- Security first: no downloaded native code on the device (`SEC-054`), isolation by
  construction, reproducible builds (`CMP-002`, `CI-006`).
- Device size: an app that does not use functions must not pay for them (`RT-060`).
- Function authors: familiar tooling, schema derivation from types (`FN-004`).

## Considered options

1. Standard Go (`GOOS=wasip1`, `go:wasmexport`).
2. TinyGo.
3. Rust (`wasm32-wasip1`).
4. AssemblyScript.

For the device engine: WAMR in interpreter mode, `wasmi`, wasmtime's Pulley interpreter.

## Decision

**Functions are written in Rust**, compiled to `wasm32-wasip1` with a pinned toolchain,
`opt-level = "s"`, LTO, `panic = "abort"`, then `wasm-opt` (`FN-001`; Binaryen `version_132`, see
[ADR-0063](0063-function-builds-provenance.md)). Everything else in
Plux stays in Go, Dart and TypeScript; Rust is used for functions only.

**Evidence.** The same function, built with Go 1.27 and Rust 1.97 (neither through
`wasm-opt`), run on `wazero` 1.11 on an x86-64 CI-class machine. The interpreter stands in
for the device engine; phones are slower for both, and the ratio is structural, because Go's
runtime and collector are in every module.

| | Go | Rust | Budget |
|---|---|---|---|
| Module, raw / gzip | 4.37 MB / 1.20 MB | 82 KB / 35 KB | ≤ 4 MiB per plugin (`FN-056`) |
| Interpreter: fresh instance | 14 ms | 0.13 ms | — |
| Interpreter: one call, p50 / p95 | 85 / 113 ms | 11 / 15 ms | ≤ 20 ms p95 on the device |
| Compiled: compile at deploy | 2.0 s | 0.05 s | ≤ 50 ms first call (`FN-023`) |
| Compiled: fresh instance | 4–6 ms | 0.25 ms | ≤ 1 ms warm overhead p99 (`FN-023`) |
| Compiled: one call | 4.6 ms | 0.8 ms | — |
| Linear memory | 4 MiB | 1.2 MiB | 32 MiB device default |
| Build, clean / incremental | 5.6 s / 0.3 s | 7.9 s / 1.8 s | — |

Go misses the device call budget by a factor of four to five in the interpreter and cannot
satisfy a fresh instance per call at the server's overhead budget. A decimal variant (1 s
against 36 ms in the interpreter) is not counted, because part of that gap was the Go code.

**Server engine.** `wazero` in compiled mode (`FN-010`). Modules are compiled when a
version is deployed and the compiled form is cached by module SHA-256; every invocation
instantiates a fresh module, so linear memory, globals and tables never survive a call.

**Device engine.** **WAMR in interpreter mode**, (version 2.4.3), built from C with the toolchain that
already builds `plux_native`, so no Rust toolchain reaches the app build. Its interpreter
subset is vendored with a SHA-256 list of every file, and a CI step compares that list with
the upstream `WAMR-2.4.3` source archive. It lives in the optional
package `plux_functions` (`RT-060`): an app that does not use functions pays nothing, and a
device-placed `invokeFunction` without the package fails with `PLX-5401`. Interpreter-only
satisfies `SEC-054`: no native code is generated or downloaded on the device.
`wasmi` (Rust, so it would put a Rust toolchain into every app build) and wasmtime's Pulley
(large, and a Rust build) are the alternatives. **Switch condition:** milestone F5 measures
WAMR and `wasmi` side by side on the emulators (library size, start, a typical call). If WAMR
misses the size or speed of `FN-056` and `wasmi` meets it, the lead presents the numbers to
the maintainer before any switch; this ADR is then revised. The numbers are recorded here
when F5 lands.

**Placement.** Each function declares exactly one placement in its document (`FN-008`). The
same Rust code and the same ABI serve both: a server function version is a module of one
function; a plugin's device functions are linked into one module (`FN-051`). A function that
needs the same logic in both places is deployed twice, once per placement; the device result
is never evidence for the server (`FN-054`).

**Related decisions.** The host ABI is language-neutral and defined separately
([ADR-0062](0062-function-host-abi.md)); builds run on the server and are signed
([ADR-0063](0063-function-builds-provenance.md)); isolation and egress are in
[ADR-0064](0064-function-isolation.md). `FN-040` (P15) becomes "Go and other languages":
because the contract is the ABI, a Go or AssemblyScript SDK is an addition, not a change.

## Consequences

- **Positive:** modules two orders of magnitude smaller; the device budget is met with
  margin even on slow phones; a fresh instance per call is cheap enough to be the isolation
  model; deploy-time compilation is negligible.
- **Negative:** Rust is a harder language than Go for the people who write functions; CI and
  the builder carry a second toolchain; the SDK and its schema derivation are our code to
  maintain (`FN-002`, `FN-004`).
- **Follow-up:** F1 shows `wasm-opt` output is byte-identical across runs before it joins
  the pinned tools (D9); F5 records the WAMR and `wasmi` numbers; the specification and the
  Studio editor requirement (`FN-033`, a Rust editor backed by `rust-analyzer`) are updated
  in the same change as this ADR.

## Options in detail

### Standard Go

Familiar to the rest of the team and complete (`reflect`, `encoding/json`). Rejected by the
measurements above: the runtime and garbage collector are in every module, which costs size,
start-up and interpreter speed together.

### TinyGo

The usual route to small Go modules. Rejected: its `reflect` support is partial and
`encoding/json` panics at run time under WASI, which removes the reason to choose Go.

### Rust

No runtime to ship, deterministic memory use, mature `wasm32-wasip1` support, strong types
for schema derivation (`schemars`), and a decimal type (`rust_decimal`) suited to money.
Costs: language difficulty and a second toolchain, accepted.

### AssemblyScript

Small modules and a TypeScript-like syntax, but a small ecosystem, no mature decimal or
JSON-Schema derivation, and weaker guarantees about memory safety. Not chosen; it remains
possible later through the ABI (`FN-040`).

### Device engines

- **WAMR, interpreter mode:** C, small, builds with `plux_native`'s toolchain. Chosen.
- **`wasmi`:** a good interpreter, but written in Rust, so every app build would need a
  Rust toolchain. Kept as the measured alternative.
- **wasmtime Pulley:** an interpreter inside a large engine; heavier than the other two and
  also a Rust build.
