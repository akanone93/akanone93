# 0063. Function builds on the server: the builder role, provenance and versions

- **Status:** Proposed
- **Date:** 2026-10-13
- **Requirements:** `FN-003`–`FN-006`, `FN-009`, `FN-016`, `FN-019`, `FN-031`

## Context and problem

A function is Rust source. Turning it into a module that runs for every tenant is the most
dangerous step in the feature: a build executes `build.rs` scripts and procedural macros, which
are arbitrary native code. A module built on a laptop proves nothing about its source. The
specification asks for provenance and signed artifacts (`FN-005`), immutable versions
(`FN-006`), reproducible builds (`FN-019`, `CMP-002`) and a builder that cannot reach the
network or the platform's secrets (`FN-016`). This ADR decides how.

## Decision drivers

- Only code Plux built and signed runs on the server (`FN-009`); no downloaded executable
  code other than sandboxed WebAssembly (`SEC-054`).
- The builder holds no database credentials and no signing keys (layering L-2, `FN-016`).
- Byte-identical output for the same input (`CMP-002`, `CI-006`).
- A developer builds locally in the same way (`plux fn build`).
- A release's behaviour is reproducible (`FN-006`).

## Considered options

1. Build on the developer's machine, upload the module, sign on upload.
2. Build on the server in a `builder` role, sandboxed; sign the result in `signing/`.
3. Build in a third-party CI service.

## Decision

Chosen option: **2**. Source and vendored dependencies are uploaded; the `builder` role
builds them and the worker has `signing/` sign the artifact. Only server builds are
deployable; `plux fn build` runs the same pipeline locally for development and tests.

**Build.** `cargo build --release --locked --offline --frozen` over `cargo vendor` sources,
`opt-level = "s"`, `lto = true`, `codegen-units = 1`, `panic = "abort"`, `strip = true`,
`--remap-path-prefix` for every path, `SOURCE_DATE_EPOCH`; then `wasm-opt -Os`. `wasm-opt` is Binaryen
`version_132`, taken from the npm package `binaryen@132.0.0`, pinned by its integrity hash and
run with the pinned Bun: one platform-independent (Emscripten) build on developer machines,
in CI and in the builder image, so the optimiser's output cannot vary by host (D9). The toolchain is pinned in `sdk/fn/rust-toolchain.toml`. Dependencies not vendored
fail with `PLX-7201`. The pipeline builds twice in CI and compares bytes (`FN-019`); a
mismatch is `PLX-7206`.

**Wrapper crate.** Every module is linked by a generated crate, `plux-module`, that depends
on one or more function crates and emits the ABI exports with a dispatch table
([ADR-0062](0062-function-host-abi.md)). It asserts each crate's placement against the
function document; a mismatch fails the build (`PLX-7205`).

**Schemas.** Built with the feature `plux-schema`, the wrapper exports `plux_schemas`. The
builder runs *that* module in `wazero` to read the input and output JSON Schemas, then
builds the artifact without the feature. User code therefore never runs natively in the
builder beyond allowlisted build scripts and procedural macros.

**Sandbox.** Each build runs in a child process in a new network namespace (loopback only),
with CPU, memory, time and source-size limits from the registry (`function.buildTimeout`,
`function.buildMemory`, `function.sourceArchiveSize`), a private mount namespace with the
vendored sources read-only and a scratch directory, and no inherited environment. Where the
kernel refuses namespaces, the builder **refuses to build**. The setting `builder.sandbox:
none` allows builds without the sandbox for development only; it is never a default and the
server logs a distinct warning on start and with every build (security default rules).

**Allowlist of build scripts and procedural macros.** A crate that has a build script or is a
procedural macro must be on an allowlist, by crate name and exact version, checked against
`Cargo.lock` and the vendored checksums. Initial list: `proc-macro2`, `quote`, `syn`,
`unicode-ident`, `serde`, `serde_derive`, `serde_json`, `schemars`, `schemars_derive`,
`rust_decimal`, `libm`, `thiserror`, `thiserror-impl`, `itoa`, `ryu`, `memchr`. Anything else
fails with `PLX-7202`. Extending the list needs the maintainer, as a change to a security
control. The sandbox is the second line behind it, not a substitute.

**Artifact statement and signature.** The builder writes the module, the schemas and the
provenance (`FN-005`: source hash, dependency list with checksums, toolchain and Binaryen
versions, build time, builder identity). The artifact statement — module SHA-256, schema
SHA-256s and provenance — is canonical JSON, signed through `signing/` with a new
**`function` key role**. The worker asks for the signature; the builder holds no key and no
database credential and hands its output to the worker (L-2). `fnrunner` verifies the
signature before it compiles a module (`FN-009`, `PLX-7019`).

**Versions and aliases.** A version is an integer per function, app and environment,
assigned at deploy and immutable (`FN-006`). Aliases match `^[a-z][a-z0-9-]*$`, as in
`FunctionGrant`, and point to versions. A release records the name, version, module SHA-256
and placement of every function its plugins reference; a device invocation sends the version
its release pinned, and the server refuses a version no active release of the app pins. A
release with an unresolved reference (`PLX-7031`) is refused.

**Tests.** Test cases stored with the function run in CI and by `plux fn test`; a function
whose tests fail cannot be deployed to production (`FN-031`, `PLX-7020`). Approval of the
production deploy uses the RBAC permission with step-up until the approval engine arrives
in P9 (D6).

## Consequences

- **Positive:** a provable chain from source to running module; a malicious build script
  meets a sandbox with no network and no secrets; versions and releases are reproducible;
  the signing key is never on the build machine.
- **Negative:** the builder is another role to deploy and size; the allowlist limits the
  crates a function can use, and extending it is a maintainer decision; builds need the
  vendored sources uploaded (large archives, bounded by the registry).
- **Follow-up:** the sandbox is Linux-specific; the Helm chart's builder pod needs the
  capability to create namespaces or a runtime class that permits it. The allowlist's
  review process is documented with `docs/functions/`.

## Options in detail

### Build on the developer's machine

Simplest, and gives no assurance: the module could come from any source. Rejected because
`FN-009` could not be enforced.

### Build on the server in a sandboxed builder

Gives provenance, a controlled toolchain and a single place to hold limits. The sandbox
works against the attacker that matters (a hostile `build.rs`), and the allowlist reduces the
number of native programs that run at all. Chosen.

### A third-party CI service

Moves the problem to a service Plux does not control and needs network access for crates;
it would make the statement unverifiable offline. Rejected.
