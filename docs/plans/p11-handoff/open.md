<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Open items and known gaps

## For the maintainer

- None open: every decision is recorded in decisions.md (2026-10-28 batch).

## Known gaps (fix before the phase exit)

- S7.5: targeting sets the segment only; one primary and one guardrail metric; flags have no per-segment values.

- Host-bundled fonts (§4.3 #11) loaded by the canvas but not wired: the server field does not exist.
- Docker image build and `make studio-image-repro` never run here (no Docker): run on a machine with Docker or in the phase-exit CI run.
- S7.2 (remaining): the visual-diff job returns only the SHA-256 of each before/after PNG and no RPC serves the bytes, so the diffs step names the stored image instead of drawing a thumbnail (needs a fetch-by-hash RPC or URL on `VisualDiffScreen`). The rollout plan form has no registry limit for the stage count because none exists.
- S8.10 (remaining): `plux test --report` records a run (one scenario result each, the failure message as one step; the Flutter harness reports scenarios, not their steps) and is opt-in, so a plain `plux test` stays offline. Studio prefills the new scenario with `tap` steps from the held device-loop session, resolved through each action trace's node path to the node's `testId`; it only sees taps of runs that arrive while the Tests page is open (the stream replays what the server retains) and only for nodes that declare a `testId`.
- Analytics funnel results count steps from daily rollups, not per-session ordered conversion.
- Mock bindings (DAT-080) cover plain `data.<source>.value|loading|error` paths (with `?.`, `.field`, `[n]`, `?? literal`); item templates and other PXL expressions are not evaluated on the canvas.
- Visual-diff renderer client (S7.1) not implemented; requested diffs stay queued (S9.5).
