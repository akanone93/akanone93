<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Function Builds and Provenance

How the source of a Plux Function becomes a module the server will run, what is recorded about
it, and why you can trust the chain. The decisions are in
[ADR-0063](../adr/0063-function-builds-provenance.md); writing and deploying functions is the
[functions guide](functions.md).

## 1. Why the server builds

Compiling Rust runs code: build scripts (`build.rs`) and procedural macros are native programs
that execute with the compiler's privileges. A module built on a laptop also proves nothing about
its source. So Plux never deploys a module you built. You upload **source and vendored
dependencies**; the `builder` role compiles them in a sandbox; the result is signed. Only
artifacts built by the `builder` role and signed with the function-artifact key are deployable,
and `fnrunner` checks the signature before it compiles a module (`FN-009`).

`plux fn build` runs the same pipeline on your machine, without the sandbox, for development and
tests. Its output is never uploaded.

```text
plux fn deploy ─ source + vendor/ ─▶ api ─▶ worker ─▶ builder   (no network, no keys, no database)
                                              │             │
                                              │◀ module, schemas, provenance
                                              ├─ signing/ signs the artifact statement
                                              └─ stores the artifact; deploy makes it a version
```

The `builder` role holds no database credentials and no signing key (layering rule L-4). The
worker asks `signing/` for the signature, with the `function` key role, and stores the result.

## 2. The build

For each build the pipeline:

1. Validates `function.json` against the document schema and the request (`PLX-7205` on a
   mismatch) and unpacks the source archive. It must be at most `function.sourceArchiveSize`
   (64 MiB by default) and carry `Cargo.toml`, `Cargo.lock`, `src/` and a `vendor/` directory made
   by `cargo vendor`. `plux fn deploy` packs all of these.
2. Wraps your crate in a generated crate, `plux-module`, that emits the ABI exports with a
   dispatch table ([ABI](../functions/abi.md)) and checks your handler's placement.
3. Resolves the dependencies from the lock file and checks every build script and procedural
   macro against the allowlist (section 4; `PLX-7202`). A dependency that is not vendored fails
   with `PLX-7201`.
4. Builds the module with the feature `plux-schema` and runs it on the function host to read the
   input and output JSON Schemas (`FN-004`). Your code never runs natively for this. If the builder cannot read the
   schemas from the module the build fails with `PLX-7204`.
5. Builds the artifact: `cargo build --release --locked --offline --frozen` for `wasm32-wasip1`
   with the pinned toolchain, Rust 1.97.0, then `wasm-opt -Os` (Binaryen `version_132`).
6. Runs your stored test cases against the module on the function host (`FN-031`).

The release profile is fixed: `opt-level = "s"`, `lto = true`, `codegen-units = 1`,
`panic = "abort"`, `strip = true`. You cannot change it in your `Cargo.toml`; the build sets it.

A build that does not compile is not an infrastructure failure: it ends in the state `failed`
with diagnostics, and `plux fn deploy` prints them. A failure of the `builder` role itself is
retried by the worker. A build is in one of `queued`, `running`, `succeeded` and `failed`.

## 3. Limits and sandbox

A build runs in a child process under limits from the registry (`LIM-001`):

| Limit | Default | Maximum | Over the limit |
|---|---|---|---|
| `function.sourceArchiveSize` | 64 MiB | 512 MiB | The upload is refused |
| `function.buildTimeout` | 10 minutes | 60 minutes | `PLX-7203` |
| `function.buildMemory` | 4 GiB | 16 GiB | `PLX-7203` |
| `function.moduleSize` | 16 MiB | 64 MiB | The build fails |
| `function.testCases` | 100 | 1000 | `PLX-7203` |
| `app.functions` | 100 | 1000 | Functions one app may have |

The sandbox puts the build in new user, network, mount and PID namespaces. The network namespace
has no interface but loopback, so a build script cannot reach anything; the environment is empty
and the scratch directory private. Where the kernel refuses these namespaces the builder
**refuses to build**. The setting `builder.sandbox: none` allows builds without a sandbox for
development only. It is never a default, and the server logs a distinct warning when it starts
and with every build.

The builder also bounds how many builds run at once (`builder.concurrency`); beyond it a request
is refused as unavailable instead of queued without end.

## 4. Allowlisted build scripts and procedural macros

A crate that has a build script or is a procedural macro must be on an allowlist, by crate name
and exact version, checked against `Cargo.lock` and the vendored checksums. A crate that is not
on it fails the build with `PLX-7202`. Only crates actually built for `wasm32-wasip1` are
checked, so an optional dependency that is never compiled does not count.

The list is `backend/internal/builder/allowlist.json`. It holds the crates the SDK needs and a
small set of widely used ones: `proc-macro2`, `quote`, `syn`, `unicode-ident`, `serde`,
`serde_core`, `serde_derive`, `serde_json`, `schemars`, `schemars_derive`, `ref-cast`,
`ref-cast-impl`, `rust_decimal`, `num-traits`, `itoa`, `memchr` and others, each at the versions
listed in the file. A crate without a build script or procedural macro, such as a pure-Rust
library, needs no entry. Adding a crate to the list is a security decision; ask the maintainers,
who change it in a reviewed commit. The sandbox is the second line of defence, not a substitute.

If your build fails with `PLX-7202`, the message names the crate and version. Choose one of: use a
different crate, pin a version that is on the list, or ask for the crate to be added.

## 5. Reproducibility

The same source, lock file and toolchain give a byte-identical module (`FN-019`,
`CMP-002`). To get there the build:

- uses vendored sources only, offline;
- remaps every path (`--remap-path-prefix`) so the module holds no host path;
- sets `SOURCE_DATE_EPOCH` and an empty environment;
- runs one pinned `wasm-opt`: Binaryen `version_132` from the npm package `binaryen@132.0.0`,
  pinned by its integrity hash and run with the pinned Bun, so the optimiser's output does not
  vary by host.

`plux fn build --verify` builds twice and fails unless both modules are equal. CI does this for
the SDK and the examples. A mismatch is `PLX-7206` (`FUNCTION_BUILD_NOT_REPRODUCIBLE`): something
in the source or its dependencies depends on the time, the path or the machine. Common causes
are a `build.rs` that embeds a timestamp or an absolute path, and a dependency pulled at a
different version.

## 6. Provenance and the signed statement

Every build records, with the module (`FN-005`):

| Field | Content |
|---|---|
| Source hash | SHA-256 over the canonical list of every source file, vendored files included |
| SDK hash | The same construction over the pinned `plux-fn` crate, which is a path crate and so absent from the lock file |
| Dependencies | The registry crates of the lock file with versions and checksums |
| Toolchain | The full `rustc --version` line |
| `wasm-opt` | The optimiser version |
| Build time | UTC, from the builder's clock; it does not enter the module |
| Builder identity | Which builder instance produced the module |

The **artifact statement** is canonical JSON holding the module's SHA-256, the SHA-256 of each
schema and this provenance. The worker has `signing/` sign it with the `function` key role.
`fnrunner` accepts a module only when:

1. the SHA-256 of the bytes equals the one requested;
2. the signature verifies against a public key in **its own configuration**
   (`fnrunner.artifactKeys`), never one that arrives from the `api` role;
3. the signed statement names that SHA-256.

Any failure is `PLX-7019` (`FUNCTION_ARTIFACT_SIGNATURE_INVALID`), and nothing is compiled. The
result is a chain you can follow from a running module back to its source: the version records the
module hash, the hash is in a signed statement, and the statement records the source hash, the
dependencies and the toolchain.

## 7. Versions are immutable

A deployed version never changes (`FN-006`); the database refuses an update of its row. A release
records the name, version, module SHA-256 and placement of every function it references, so a
release always runs the code it was tested with. To change a function, deploy a new version and
move an alias or publish a new release. See [versions and promotion](functions.md#7-promote-between-environments).
