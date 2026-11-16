<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Building the Apply Register app end to end (maintainer request, 2026-11-16)

The maintainer's build sheet (`ALL-IN-ONE-APP.md`, sections A–O) is built in Studio as a person would, on the
local stack, with a designed look, and every defect found on the way is logged below, root-caused and fixed
(small ones decided here, big ones put to the maintainer). Progress screenshots go to the maintainer as the build
advances.

## Order of work

1. Decisions of 2026-11-16 — done as each lands:
   - [x] Device compare: no light/dark check; the developer sets both sides alike.
   - [x] Translation catalogue check with `--overwrite` wherever it runs (`mk/studio.mk`, `studio/AGENTS.md`).
   - [x] Restrictions: pick the team or person instead of typing an identifier.
   - [x] Small walk findings: a not-found page inside the app frame; the copy after adding a key.
   - [x] Translation keys referenced only by their immutable identifier everywhere (investigate every use).
   - [x] Deleting a key removes its messages from every locale properly (investigate first).
2. `ar-api`: a stand-alone Go service with handlers only — in-memory data, a minimal JWT flow, REST, GraphQL,
   WebSocket and SSE as §A1 lists, no tests — delivered zipped (not in this repository).
3. The app, section by section (B → J), each screen designed (tokens, colours, radii, structure) and checked on
   the canvas in every preview mode; screenshots sent as it grows.

## Issues found

Numbered in the order found. State: `open`, `fixed (<commit>)`, `decision`.

| # | Where | Issue | State |
|---|---|---|---|
| 1 | Router | An unknown URL shows a bare "Page not found" outside the app frame, with no way back. | fixed (shell layout's not-found page with a way home) |
| 2 | Profile › Security keys | After adding a key the message says the session "counts as signed in with it", yet the next protected action still asks for a fresh confirmation. | fixed (copy: the key signs you in; protected actions ask for it again) |
| 3 | Administration › Restrictions | A restriction's target is a team or person identifier typed by hand. | fixed (one "Applies to" choice of the organisation, a team or a person, by name; the list shows names) |
| 4 | Plugin canvas › Insert | Inserting a widget into a selected Scaffold leaves the Scaffold selected; the new widget is neither selected nor visible (an empty ElevatedButton draws nothing). | open |
| 5 | Plugin canvas › Overview | The plugin editor's header counts 11 widgets while the Overview card counts 8. | open |
| 6 | Plugin canvas › link menu | Screens with the same title could not be told apart (fixed in 24a13bad: the route is added). | fixed (24a13bad) |
| 7 | Translation catalogues | `lingui extract` keeps the English text of a changed explicit-id message, so a changed string never shows and its translations go stale unnoticed. | fixed (`--overwrite` in the gate and the recipe) |
| 8 | PXL `t()` | `t()` named a key by its name, against I18N-007 (documents use `{"$t": id}`), so a rename would break it; and it never worked: neither the runtime nor the server gave PXL a translate function, so every `t()` failed when evaluated. | fixed (`t(keyId, args)`: the compiler resolves the identifier and reports a name or unknown id as PLX-1102; the runtime formats the key's message by identifier; spec table, stdlib reference and guide say so) |
| 9 | Studio › where used | Studio's key usage (Used in, the delete guard of STU-072) saw only `{"$t": id}`, so a key read by an expression could be deleted while in use. | fixed (expressions' `t("<id>")` count as uses) |
| 10 | Studio › expression editor | Nothing helped write `t()`; with identifiers it would be unusable. | fixed (inside `t("…")` the editor offers the keys by name and inserts the identifier; a chip shows each identifier's key name, "?" when unknown) |
| 11 | Localisation › keys | A key could not be renamed. | fixed (rename in place; only the name changes, as every reference is by identifier) |
| 12 | Localisation › delete | Deleting a key left its messages in the other locales' documents. Investigated: locks allow the draft holder (an app manager) to write every locale; undo is client-side, so a server-side cascade would make undo lose text. | fixed (one undo step removes the key's message and review state in every locale; undo restores all; releases still leave out any message without a key) |
