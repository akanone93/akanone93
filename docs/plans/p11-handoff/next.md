<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Next steps, in order

Models per [agent-rules.md](agent-rules.md). Each brief uses the template there.

1. **Lead: merge `handoff/p11-session2` into `feat/phase-11`**, push.
2. **Lead: merge `p11/s55c-app-lock`** (only `.po` conflicts), run `make studio-check` and
   `go test -race ./internal/document/... ./internal/api/...`, push.
3. **Lead: review `p11/s54b-rust-editor` (WIP).** Read the diff (security: the LSP session cap in the
   shared cache, method allowlist untouched, CSP of the overlay). Then one **Sonnet low** follow-up
   in its worktree to finish what is missing and run: `go test -race` on `internal/function`,
   `internal/server`, `internal/api`; `make go-lint gen-check studio-check studio-e2e`. Then merge.
   Afterwards wire the S5.6 Code tab to open the Rust editor and the test runner.
4. **Lead: review `p11/s46c-rest` (WIP).** One **Sonnet low** follow-up: make the 10 ports match their
   goldens (`make studio-conformance-update`, then `make studio-conformance` 0 fail), shrink the
   missing list in `widgets/screen.test.ts` to zero, then the golden-level matrix (S4.6 row:
   descriptors × frames × locales incl. `ar` and the complex scripts × text scales; where the canvas
   cannot match a script, the authoritative-device notice). Then `make dart-analyze reuse-lint`.
5. **S5.6 gaps — Haiku or Sonnet low:** restore through the `RestoreSnapshot` RPC (proto
   `document.proto:51`) so documents the plugin no longer has come back; State tab forms; editor
   session tags (§4.3 #25) in the history list.
6. **S5 exit gate (lead):** touched packages' tests, `make studio-e2e`, `make studio-design-gate`, the
   slice's end-to-end flow (open each overlay V1–V9 under the CSP), open items fixed or put to the
   maintainer ([open.md](open.md)).
7. **Wave 8** (plan §6.1): S6.1–S6.5 (library, clipboard, preview, device loop) and S7.1 (release
   request server work starts early). Read the rows in [p11.md](../p11.md) §6, write briefs, Haiku/Sonnet
   low by default.
8. Waves 9–11 as in the plan, then the phase exit (S9.7): statuses in `docs/requirements.md`,
   `make trace`, `make check`, benchmarks, one CI run on the PR (M-6).
