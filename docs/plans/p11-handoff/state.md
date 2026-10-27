<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# State at hand-off (2026-10-27)

## Branches

| Branch | State |
|---|---|
| `feat/phase-11` @ `daa1d1c8` | All merged work, pushed. Gates green on this head (see [verification.md](verification.md)). |
| `handoff/p11-session2` | `feat/phase-11` + this package + the work-log note + the model-tier revision (M-10, sub-agents.md). Merge it into `feat/phase-11` first. |
| `p11/s55c-app-lock` @ `3f0ec5aa` | **Finished, reviewed by its agent, not merged.** App documents (`theme.json`, `app.json`) edit under the app lock; importer `variables` merge into `app.json`; Postgres test `TestAppDocumentPatchNeedsTheAppLock` (the server already refused plugin-lock writes — the feature was broken, not a bypass). Its merge into `feat/phase-11` conflicts only in the three `.po` catalogues — use the recipe in [lead.md](lead.md). |
| `p11/s54b-rust-editor` @ `ae06cf5d` (WIP) | **Agent stopped mid-way; not reviewed, not verified.** 27 files: Rust editor overlay (`features/rust-editor/`), `OverlayId` "rust", LSP providers wired to the bridge, per-organisation LSP session cap moved to the shared cache (`internal/function`, `internal/server`), API integration tests, e2e `studio/e2e/rust-editor.spec.ts`, catalogues. Review, finish, run its gates. |
| `p11/s46c-rest` @ `bd549f9d` (WIP) | **Agent stopped mid-way; not reviewed, not verified.** 98 files: scenes and Flutter goldens for Autocomplete, MaterialBanner, BottomAppBar, CalendarDatePicker, DataTable, DropdownMenu, NavigationDrawer, PopupMenuButton, SegmentedButton, Stepper (`src/scene/material-more.ts`, `test/layout-conformance/test/material_composites.dart`). Unknown whether the TS ports match their goldens; the golden-level matrix was not started. Regenerate goldens and run `make studio-conformance` before trusting it. |
| `p11/s13-gap`, `p11/s13-tiles` | Old canvas experiments, deliberately not merged (superseded by the CPU raster backend). Leave them. |

Worktrees of the three open branches are under `.claude/worktrees/` in the old container only; a
new session recreates them from the pushed branches.

## Done in session 1 (merged on `feat/phase-11`)

- **S0–S2** (waves 1–5): foundations, canvas go/no-go and CPU raster on software GL, layout core and
  conformance, BFF (CSP nonce, Trusted Types `plux-worker`, `__Host-` session, shared OIDC state
  secret), shell, delivery, dev sessions on the shared stream layer, editing core (JSON Patch
  autosave, locks L-1, undo with editor sessions, uploads), selection, panels, screens.
- **S3** (wave 6): app canvas C1 on Plux Canvas (migration 0057 metadata, org-scoped team/icon
  checks, `ListPluginSummaries`), bundled arrows (drag p95 16.67 ms, lift 5 ms, drop settle ≤ 267 ms
  at 200 boxes/1,000 links), ghost screens, link creation, safety rules and `ApplyChangeSet`
  (multi-document, one undo step), trash/deprecation, S3 exit gate (gating design gate,
  `e2e/s3-flow.spec.ts`).
- **S4**: slivers, boxes, all Material and Cupertino composites, all 58 Layer 2 components with
  goldens written by the real runtime (`packages/plux_flutter/test/render/layout_goldens_test.dart`),
  canvas builders for 196 of 207 widget types (10 left: see `p11/s46c-rest`), smoke matrix
  (`widgets/matrix.test.ts`), host-font loading in `canvas/src/fonts.ts` (not wired: no server field).
- **S5** (wave 7): overlay host and Monaco JSON editor (STU-123 p95 54 ms of 150), PXL language
  service in the validator WebAssembly with CodeMirror inline fields, action graph editor
  (`@xyflow/react` 12.12.0), localisation table and key extraction (I18N-007), state designer, data
  source designer with OpenAPI/GraphQL import in WebAssembly, DB schema designer with migration
  preview, theme editor, history overlay V9, remaining panel tabs, LSP bridge (rust-analyzer in the
  builder sandbox) and test runner (FN-034) on the server.
- Tooling fixed on the way: TS registry emits `translatable`; tool tests updated for the canvas
  document kind and the Studio CI jobs; Dependabot covers `studio/` (docker) and
  `test/layout-conformance` (pub); Monaco loads the editor core and JSON only (TypeScript/CSS/HTML
  workers gone) and its `<style>` elements get the page nonce through a pinned Vite transform;
  zero spacing token so `inset-0` exists; React StrictMode double mount no longer kills the canvas
  host.

Migrations used: 0054–0057. Next free migration: **0058**.
