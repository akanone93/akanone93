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
- `TermsViewer` uses a fixed 480 px design-time height (no device height in the screen theme);
  `calendar-tables.ts` is a test-pinned copy of `schema/l10n/calendars/*.json`, not generated.
- Material MenuAnchor not ported (PopupMenu instead); NavigationRail has no screen builder.
- Docker image build and `make studio-image-repro` never run here (no Docker): run on a machine with Docker or in the phase-exit CI run.
- S7.2: rollout plan is a JSON textarea; diffs step has no thumbnails/JSON diff; no per-plugin a11y/size reports.
- S8.10: `plux test` does not call `ReportRun`; steps are not prefilled from the device loop.
- S8.4: the A19 adopt dialog and A16–A18 dialogs lack tests.
- Analytics funnel results count steps from daily rollups, not per-session ordered conversion.
- Mock bindings (DAT-080) cover plain `data.<source>.value|loading|error` paths (with `?.`, `.field`, `[n]`, `?? literal`); item templates and other PXL expressions are not evaluated on the canvas.
- Visual-diff renderer client (S7.1) not implemented; requested diffs stay queued (S9.5).
