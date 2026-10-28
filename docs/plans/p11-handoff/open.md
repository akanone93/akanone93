<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Open items and known gaps

## For the maintainer

- None open: every decision is recorded in decisions.md (2026-10-28 batch).

## Known gaps (fix before the phase exit)

- S7.5: targeting sets the segment only; one primary and one guardrail metric; flags have no per-segment values.

- Preview mock states (DAT-080) are selectable, but the canvas scene has no data bindings yet.

- History restore through `RestoreSnapshot` brings deleted documents back outside the client's
  undo step (the server writes them).
- Localisation key extraction: one server undo step, but each document's local undo stack undoes
  only its part.
- Action graph: node positions not saved; no drag from the catalogue.
- Host-bundled fonts (§4.3 #11) loaded by the canvas but not wired: the server field does not exist.
- `TermsViewer` uses a fixed 480 px design-time height (no device height in the screen theme);
  `calendar-tables.ts` is a test-pinned copy of `schema/l10n/calendars/*.json`, not generated.
- Material MenuAnchor not ported (PopupMenu instead); NavigationRail has no screen builder.
- Docker image build and `make studio-image-repro` never run here (no Docker): run on a machine with Docker or in the phase-exit CI run.
- S7.2: rollout plan is a JSON textarea; diffs step has no thumbnails/JSON diff; no per-plugin a11y/size reports.
- S8.10: `plux test` does not call `ReportRun`; steps are not prefilled from the device loop.
- S8.4: the A19 adopt dialog and A16–A18 dialogs lack tests.
- Session replay A47 (O-8, ANL-050): built (ADR-0101): recorder, redacted ingestion, retention, list and viewer. The viewer draws the recorded layout in SVG, not through the CanvasKit worker; moving it onto the canvas worker is open.
- Analytics funnel results count steps from daily rollups, not per-session ordered conversion.
- No app-area navigation: releases, functions, data, localisation, design, analytics, tests, experiments, approvals, publish, devices/security and settings are reached only by URL or in-page links.
- Analytics "Open node" passes the error's `nodePath` ("root/0" form) as the canvas `node` param; unverified that it maps to a document node id.
- Visual-diff renderer client (S7.1) not implemented; requested diffs stay queued (S9.5).
