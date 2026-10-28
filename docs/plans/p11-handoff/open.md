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
- S8.10: `plux test` does not call `ReportRun`; steps are not prefilled from the device loop.
- S8.4 (remaining): the A16 brand-overlay dialog and the A18 icon form have validation tests but no server-refusal test: they send no RPC themselves (edits go through the autosave pipeline) and the test upstream cannot refuse `PatchDocument`.
- Analytics funnel results count steps from daily rollups, not per-session ordered conversion.
- Mock bindings (DAT-080) cover plain `data.<source>.value|loading|error` paths (with `?.`, `.field`, `[n]`, `?? literal`); item templates and other PXL expressions are not evaluated on the canvas.
- Visual-diff renderer client (S7.1) not implemented; requested diffs stay queued (S9.5).
