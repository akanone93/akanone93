<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Open items and known gaps

## For the maintainer

- **Design-gate rules switched off** for approved tokens (decisions.md) — revisit with the tokens.

## Known gaps (fix before the phase exit)

- Functions area (S8.2): `FunctionVersion` has no granted-capabilities or required-assurance fields and no source id, so A11 shows placement/ABI/hashes and the tests tab takes a typed source id; the logs tab has no env dropdown.
- S7.5: targeting sets the segment only; one primary and one guardrail metric; flags have no per-segment values.

- Clipboard "copy over" (F7, D34) keeps the reference only: the envelope carries IDs, so no
  dependency document is created in the target. Needs a decision on fetching the source content.
- Preview mock states (DAT-080) are selectable, but the canvas scene has no data bindings yet.

- History restore through `RestoreSnapshot` brings deleted documents back outside the client's
  undo step (the server writes them).
- Localisation key extraction: one server undo step, but each document's local undo stack undoes
  only its part.
- Action graph: node positions not saved; no drag from the catalogue.
- Host-bundled fonts (§4.3 #11) loaded by the canvas but not wired: the server field does not exist.
- `TermsViewer` uses a fixed 480 px design-time height (no device height in the screen theme);
  `calendar-tables.ts` is a test-pinned copy of `schema/l10n/calendars/*.json`, not generated.
- Scene C drop frame is 117 ms in the worker (arrows rerouted at drop end); no documented target.
- Duplicate plugin rewrites top-level ids only; the D17 owning-team picker is missing; frame
  rename/resize (D20) missing on the app canvas.
- Material MenuAnchor not ported (PopupMenu instead); NavigationRail has no screen builder.
- Docker image build and `make studio-image-repro` never run here (no Docker): run on a machine with Docker or in the phase-exit CI run.
- Clipboard copy-over decision (lead, 2026-10-28): fetch the source documents through the API with the user's own read access at paste time; what they cannot read is dropped and listed.
- S7.2: rollout plan is a JSON textarea; diffs step has no thumbnails/JSON diff; no per-plugin a11y/size reports.
- S8.10: `plux test` does not call `ReportRun`; steps are not prefilled from the device loop.
- S8.4: the A19 adopt dialog and A16–A18 dialogs lack tests.
- Session replay A47 (O-8, ANL-050): ADR-0101 was never written; recorder, redacted ingestion and the canvas viewer are not built.
- Analytics funnel results count steps from daily rollups, not per-session ordered conversion.
- Visual-diff renderer client (S7.1) not implemented; requested diffs stay queued (S9.5).
