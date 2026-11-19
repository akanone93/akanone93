<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0105. Function dependencies travel as verified crate archives, stored once, and the allowlist pins their contents

- **Status:** Accepted (maintainer, 2026-11-18)
- **Date:** 2026-11-18
- **Requirements:** `FN-003`, `FN-005`, `FN-016`, `FN-019`, `LIM-001`
- **Amends:** [ADR-0063](0063-function-builds-provenance.md) (what is uploaded and how the allowlist identifies a crate)

## Context and problem

Under ADR-0063 a function's source was uploaded with a `vendor/` directory made by
`cargo vendor`, and the builder trusted it. Building the Apply Register app (13 functions)
showed three problems, which the maintainer ranked as blockers (build log #159–#161):

1. **Security.** The allowlist of build scripts and procedural macros identified a crate by
   name and version only, and the checksums it was "checked against" — `Cargo.lock` and
   `vendor/*/.cargo-checksum.json` — are written by whoever uploads. A modified copy of an
   allowlisted crate, with its checksum file recomputed, passed: its build-time code ran
   natively in the builder, and the provenance named the genuine crates.io crate. A
   throw-away test proved it with `serde 1.0.229`'s `build.rs`. The sandbox (no network, no
   keys, no database) held, but a stated control did not, and the provenance could mislead.
2. **Storage.** Every deploy stored its whole source archive, 1.2–2.3 MB of which almost all
   was the same dependency source: 13 functions kept 13 copies of `serde`, `syn` and the rest,
   in every environment and version. On the developer's machine every function kept its own
   9–19 MB `vendor/` copy.
3. **Speed** (ADR to follow): every module build compiles every dependency from scratch.

## Decision drivers

- Security first: a pinned crate must be the published code and nothing else; provenance must
  state only what was verified (spec §1.4).
- No reduction in any existing control: builds stay offline, locked, sandboxed and
  reproducible; the builder still holds no credentials (L-4).
- Each dependency crosses the network and is stored once, as small as it comes.
- Additive and explicit: the format change is called out before the first release (0.0.1).

## Considered options

1. **Pin the vendored trees.** Keep uploading `vendor/`; pin a hash over each allowlisted
   crate's files. Closes the allowlist hole, but trees are uploaded and stored per function,
   and hashing a file tree depends on what each Cargo version copies.
2. **Upload the published `.crate` archives; verify, store once, build vendor/ in the
   builder.** Every archive's SHA-256 is the checksum `Cargo.lock` records and crates.io
   publishes. The allowlist pins that checksum.
3. **Document only.** State that the allowlist trusts the uploader. Rejected for a financial
   platform.

## Decision

Chosen option: **2**.

**What travels.** A function source is its crate only — `Cargo.toml`, `Cargo.lock`, `src/`,
`function.json`; a `vendor/` directory is refused (`PLX-7201`). Its dependencies are the
`.crate` archives of the crates.io packages its `Cargo.lock` names. `plux fn deploy` runs
`cargo fetch`, reads each archive from Cargo's own cache by checksum, asks the server which
the organisation lacks (`ListMissingCrates`) and sends only those (`UploadCrate`). The server
keeps an archive only if its bytes have the SHA-256 it was sent as.

**Where it is stored.** An archive is stored once in the object store under its SHA-256
(`crates/<sha256>`), whichever organisation sent it: the bytes are public crates.io releases
verified by content address, so sharing the bytes reveals nothing and cannot carry one
tenant's change to another. A row per organisation (`function_crates`, row-level security)
says which archives an organisation sent; it is told about, and builds with, only those. A
stored source records the archives its lock file names (`function_source_crates`), so every
build, link and editor session sends exactly those. `UploadSource` refuses a source whose lock
file names an archive the organisation has not sent, more than `function.sourceCrates`
archives (new registry limit, default 512), or a source that with its archives is above
`function.sourceArchiveSize`. The first upload of an archive by an organisation is audited
(`function.crate_uploaded`).

**How it is built.** The builder receives the archives with the source and checks each one's
SHA-256 against `Cargo.lock` — again, whatever the server checked. It refuses an archive the
lock file does not name, one given twice, one not in gzip/tar form, any entry outside
`<name>-<version>/`, any path that is not clean, any entry other than a file or directory, a
checksum file inside the archive, and a manifest that names another package or version. It
then writes `vendor/<name>-<version>/` itself, with the `.cargo-checksum.json` Cargo verifies,
computed from the bytes it extracted. Cargo then builds as before: offline, locked, from that
directory only.

**The allowlist pins contents.** `allowlist.json` maps each crate to versions and the SHA-256
of the published archive: `{"serde": {"1.0.229": "4148…"}}`, equal to `sdk/fn/Cargo.lock`
(a test checks) and to the crates.io index (checked when written). A crate with a build script
or procedural macro that the build compiles must (a) come from crates.io — a path or workspace
package never runs code at build time — (b) be a listed version, and (c) have exactly the
pinned checksum; otherwise `PLX-7202`. Because vendor/ is made only from archives whose SHA-256
is the lock file's checksum, (c) means the code that runs is the published code.

**Clean-up.** A per-organisation job removes the organisation's archives that no stored source
names and that nobody needed for a week (`last_used_at`, set when an archive is sent, when a deploy
is told the organisation holds it, and when a source names it), and removes an archive's bytes
from the object store only when no organisation holds it any more — checked in the read-only
`crate_store` scope, under a per-archive lock that the upload takes too, so bytes being stored are
never removed. Each run that removes archives is audited (`function.crates_purged`).

**Provenance.** The dependency list keeps name, version and checksum; each checksum is now the
SHA-256 of the archive actually built, verified by the builder.

**Developer machines.** `plux fn` keeps no copy of dependencies in a function: Cargo's cache
is the one copy, shared by every project. An old `vendor/` written by `cargo vendor` is
removed. Native tests of all functions share one build directory, `functions/.target`.

## Consequences

- **Positive:** the allowlist is a real control; provenance states verified facts; each
  dependency is uploaded and stored once (source archives that were 1.2–2.3 MB with their
  `vendor/` are now a few kilobytes: the fee example's is 4,318 bytes);
  no per-function copies on developer machines.
- **Negative:** an incompatible change to the upload format and the internal builder
  protocol (called out; allowed before 0.0.1); sources stored before this change cannot be
  rebuilt — their deployed, signed versions keep running and can be promoted, and the next
  deploy uploads the new form. The API process now parses the lock file of an uploaded
  archive (bounded, with the builder's hardened extractor).
- **Follow-up:** module builds reuse compiled dependencies — locally first, then per
  organisation once builds are isolated from one another on the builder's filesystem
  (build log #161, #164, own ADR).

## Options in detail

### Option 1 — pin the vendored trees

Closes the hole for allowlisted crates only; non-allowlisted crates' provenance stays
uploader-asserted; trees are uploaded and stored per function; a tree hash is fragile across
Cargo versions, which copy or skip different files.

### Option 2 — published archives, verified and stored once

One identity per crate version that crates.io itself publishes; verification is a hash; the
builder never trusts an uploaded tree; storage and upload shrink to one compressed archive per
crate version. Costs a protocol change and a crate store.

### Option 3 — document only

Keeps the hole; rejected.
