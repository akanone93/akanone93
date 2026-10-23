<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0089. Incremental compilation and hot apply

- **Status:** Accepted (maintainer, 2026-10-22; P10 plan §2.2, §5.1, §5.3)
- **Date:** 2026-10-22
- **Requirements:** `CMP-051`, `CMP-002`, `DEV-010`, `NFR-030`, `SEC-052`

## Context and problem

The edit-to-device budget is 500 ms p95 (`NFR-030`). Today the compiler is one-shot over a whole
unit: it caches nothing per page and does not know which pages an edit affects. The device must
then apply the new sections without losing the state a developer has built up by hand, which
needs to know whether the state schema of a page changed. The compiler is a pure library (L-5)
and its output is byte-identical for the same input (`CMP-002`). The question: how does the
compiler recompile only what an edit touches, and how does the runtime apply the result?

## Decision drivers

- Determinism: incremental output equals a full compile, byte for byte; no wall clock, no map order, no environment data may reach an artifact.
- Purity (L-5): no I/O in the compiler; the cache is injected.
- Latency: a one-page edit of a 50-page plugin compiles in at most 100 ms p95 on the CI runner; the stage budgets are server ≤ 120 ms and device apply ≤ 150 ms.
- Nothing from the UI isolate: patching and verification run in a background isolate (L-6).

## Considered options

1. A `Cache` value owned by the caller, keyed by content hashes and the reference graph; the result carries a per-page state-compatibility verdict that the runtime uses to keep or reset state.
2. Full recompilation on every edit, optimised in place.
3. A persistent on-disk cache inside the compiler.
4. Runtime hot reload that always resets page state.

## Decision

Chosen option: **1**, because it is the only one that meets the budget without giving up
determinism or purity.

- **Cache.** `backend/internal/compiler/incremental` defines `Cache`, owned by the caller (the
  worker or the CLI) and passed in. An entry is keyed by the content hash of a document plus the
  hashes of everything it references, taken from the reference graph (`compiler/refgraph.go`).
  An edit invalidates the edited document and its transitive dependants (widgets, components,
  themes, translations, data sources, state schema, navigation) and nothing else (`CMP-051`).
  Keys are content hashes only: never a time stamp, a path or an iteration order.
- **Equality with a full compile.** A property test compiles a generated edit sequence both ways
  and compares the bytes (`CMP-002`), and a unit test covers each dependency edge. The fuzzed
  run belongs to the determinism gate.
- **Result.** The compile returns the changed sections and, per page, a state-compatibility
  verdict: the same state schema, or the list of incompatible scopes.
- **Hot apply.** The runtime verifies the dev bundle (ADR-0087), applies the changed sections to
  the session overlay and re-renders the changed pages. It keeps the state scopes the verdict
  marks compatible, resets the others and says so in the stream with `PLX-9231`.
- **First measurement.** The latency of a one-page edit of a 50-page plugin is measured in the
  compiler milestone, before the service is built, so that regressions are attributable;
  `make bench-devloop` reports the server part separately from the total (`NFR-030`).

## Consequences

- **Positive:** the budget is reachable; the cache is a plain value that tests can fill and
  inspect; state survives most edits.
- **Negative:** the reference graph must be complete — a missing edge is a stale page, which the
  property test exists to catch; the verdict is conservative, so some edits reset state that
  could have been kept.
- **Follow-up:** the `incremental` package with a coverage floor of 80 %; the verdict's wire form
  in `devsession.proto`; the same milestone closes the P9 sync-size baseline (D-10).

## Options in detail

### Option 1 — injected cache and verdict

Described above.

### Option 2 — optimise the full compile

Needs no cache, but cost grows with the plugin, so the budget fails on large plugins, and it
gives the runtime no way to know what changed.

### Option 3 — persistent cache in the compiler

Puts disk I/O in a pure library (L-5) and makes the output depend on what an earlier run left
behind. Rejected.

### Option 4 — always reset state

Simplest, but a developer then re-creates the state after every save, which defeats the point of
a fast loop.
