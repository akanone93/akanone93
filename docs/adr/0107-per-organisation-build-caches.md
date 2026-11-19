<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0107. Server builds reuse the compiled dependencies of their own organisation, and never a compiled function

- **Status:** Accepted (maintainer, 2026-11-18, build log #161; written 2026-11-19)
- **Date:** 2026-11-19
- **Requirements:** `FN-016`, `FN-019`, `LIM-001`
- **Builds on:** [ADR-0105](0105-function-dependencies-as-verified-crates.md) (verified crate
  archives), [ADR-0106](0106-builds-isolated-by-a-private-view-and-landlock.md) (build isolation)

## Context and problem

Every server build compiled every dependency from scratch: about 12 s in the builder role for a
function whose own code compiles in a second (build log #161). The maintainer chose to share
compiled dependencies per organisation once builds were isolated from one another (#164,
ADR-0106). A cache is shared, writable state between builds, so it must not:

- let one organisation's build read or change another organisation's;
- let a module be made from anything but the source its provenance names — a signed module
  whose source an auditor reads must be the module that source compiles to (`FN-019`);
- grow without bound on the builder's disk.

Measured while designing it: Cargo decides whether a compiled crate is up to date from file
times for path packages (the function crates, the module wrapper, the SDK). With every file of a
cache dated in the future, a changed function source was not compiled again and the build
returned the module of the earlier source (a real-toolchain test proves it). The builder writes
every file itself with the current time, so this does not happen in normal operation, but the
property must not rest on file times.

## Decision drivers

- Security first: no cross-tenant reach; provenance states only what was built.
- Performance: the dependencies, which are most of a build, compile once per organisation.
- Bounded: every size comes from the registry or the deployment (`LIM-001`).

## Considered options

1. **No server cache.** Safe and slow.
2. **One cache per organisation, keeping only crates.io dependencies** (this decision).
3. **One cache for every organisation.** The crates.io dependencies are the same bytes for
   everyone, but a shared writable directory is a channel between tenants, and a build script
   pinned for one is code that runs beside another's output. Rejected.

## Decision

Chosen option: **2**.

- **Keyed caches.** The worker sends the build's organisation ID as `cache_key` with every
  build and link (`BuildRequest`, `LinkRequest`); the build was claimed in that organisation's
  tenant scope, so the key is the source's organisation. The builder keeps one cache per key
  below `builder.cacheDir`: the workspace, Cargo home and target directory at paths that do not
  change. A key must be 1 to 64 lower-case letters, digits and hyphens; any other key, or none,
  builds from scratch. The cache is writable to that build's tools beside their scratch
  directory (`Command.Writable`); every other cache is outside the build's view and its Landlock
  rules (ADR-0106).
- **Only verified dependencies are reused.** Before compiling, a build with an organisation's
  cache runs `cargo clean` for every path package of its lock file — the functions, the module
  wrapper and the SDK — so they are compiled from this build's sources whatever any file's time
  says. What stays are crates.io crates, each made from an archive whose SHA-256 is its
  `Cargo.lock` checksum (ADR-0105), and their build-script outputs.
- **One build at a time per cache.** A build takes its cache's lock; a second build of the same
  organisation meanwhile builds from scratch.
- **Bounded.** One cache's target directory is emptied when a build leaves it above
  `function.buildCacheSize` (registry, default 2 GiB, at most `builder.cacheBytes`). After each
  build, the caches used least recently are removed until all fit in `builder.cacheBytes`; a
  cache in use is never removed, and a cache is renamed under its lock before it is removed, so a
  build that starts meanwhile makes a new one.
- **Deployment.** Compose: a 3 GiB executable `tmpfs` with `builder.cacheBytes` 2.5 GiB.
  Helm: `builder.cache.sizeGi` (default 8) on the node's disk, the caches kept to 90% of it;
  0 turns the cache off. Both are gone with the container or pod.
- **Developer machines** keep their single unkeyed cache (`function.devBuildCacheSize`) and do
  not clean the path packages: their modules are not signed, and Cargo's usual reuse is faster.

## Consequences

- **Positive:** measured on the development stack, a function's build in the builder role took
  5.5 s instead of 11.6 s and the whole deploy 7.7 s instead of 15.8 s, with a byte-identical
  module; one organisation's cache was 77 MB.
- **Negative:** within one organisation, a later build uses dependencies an earlier build
  compiled. Only pinned, published build code (the allowlist) and the compilers run while they
  are compiled, so changing them needs a compiler exploit or a malicious pinned crate, and the
  result stays within the organisation. A real-toolchain test proves that a module is never
  made from a stale compilation of a function, even with every cached file dated in the future.
- **Follow-up:** none.
