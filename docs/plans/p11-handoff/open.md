<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Open items and known gaps

## For the maintainer

- **Design-gate rules switched off** for approved tokens (decisions.md) — revisit with the tokens.

## Known gaps (fix before the phase exit)

- History restore skips documents the plugin no longer has (use `RestoreSnapshot`); no State tab
  forms; editor-session tags (§4.3 #25) unused.
- Localisation key extraction: one server undo step, but each document's local undo stack undoes
  only its part.
- Theme/data designers: data source `route`, `config`, `transaction` and collection `indexes`
  editable only in the JSON editor.
- Action graph: node positions not saved; no drag from the catalogue.
- Host-bundled fonts (§4.3 #11) loaded by the canvas but not wired: the server field does not exist.
- `TermsViewer` uses a fixed 480 px design-time height (no device height in the screen theme);
  `calendar-tables.ts` is a test-pinned copy of `schema/l10n/calendars/*.json`, not generated.
- Scene C drop frame is 117 ms in the worker (arrows rerouted at drop end); no documented target.
- Duplicate plugin rewrites top-level ids only; the D17 owning-team picker is missing; frame
  rename/resize (D20) missing on the app canvas.
- Material MenuAnchor not ported (PopupMenu instead); NavigationRail has no screen builder.
- Possible flaky unit test: the D27 rename-warning test failed once under full-suite load
  (the JSON editor tests got a 5 s boot wait for the same reason). Root-cause before quarantining.
- Docker image build and `make studio-image-repro` never run here (no Docker).
