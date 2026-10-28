<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Brief: GEN-004 completion — shell icon and project patch

Lead setup: `git worktree add -q .claude/worktrees/g-shell -b p11/g-shell feat/phase-11`.
Model: Sonnet (general-purpose agent, background). Maintainer decision 2026-10-28: implement in P11
(additive proto fields, pre-0.0.1).

```
You work in the git worktree /home/user/akanone93/.claude/worktrees/g-shell (branch p11/g-shell). Run every
command there; first `cd studio && bun install --frozen-lockfile`. Read AGENTS.md §5–§7, backend/AGENTS.md,
studio/AGENTS.md, docs/requirements.md row GEN-004, docs/adr/0024-no-code-generated-projects.md (§ShellSpec,
§Icons), proto/plux/v1/app_project.proto (ShellSettings, GenerateProject), backend/internal/shellupdate/,
backend/internal/generator/generator.go (Input.Icon, ShellSpec.IconSHA256), and
studio/apps/web/src/features/shell-settings/. Act at once: no plan. Decide small technical points (safest
option) and note them; stop only for a security default, new dependency or new limit.

Lead's decisions (follow them):
1. Icon. Additive `string icon_asset_id = 14;` on ShellSettings: an existing asset of the same app (asset
   service, image/png). SetShellSettings checks it under the app's RLS (another app's or organisation's asset →
   invalid argument, never a leak of its existence) and the generator's icon rules (square PNG, 1024–4096 px;
   reuse the generator's decode check, no new limit). The job loads the asset bytes into generator.Input.Icon,
   so ShellSpec.IconSHA256 is recorded and the shell comparison (compare.go) reports an icon change with a
   reason. Studio: an icon picker in the shell form from the app's PNG assets (ListAssets), with preview and
   "default icon".
2. Patch. GenerateProject accepts format "patch" (format stays a string; additive). It needs a previous
   successful generation of the app (else failed precondition with a registered error code): generate the
   project from the stored generated spec (and the icon with the stored IconSHA256 — load it by content hash from
   the asset store; if no longer available, failed precondition, registered code) and from the current spec,
   diff the two file maps, and write a deterministic zip holding the added/changed files plus
   `plux-patch.json` (sorted lists: changed, added, deleted paths with sha256, from/to ShellSpec). Byte-identical
   for the same inputs (CMP-002 spirit). Studio: the shell page offers "Download patch" beside "Regenerate
   project" when a generation exists and an update is required, and shows what changed (already present).
Tests: Go unit + integration (test database, -race) for icon validation including cross-app/cross-org asset,
icon change detection, patch content, determinism and the precondition errors; component tests with [GEN-004]
in titles; Lingui en/de/ar complete. Register any new error code per docs/engineering/error-handling.md. Set
GEN-004 to DONE only if name, icons, splash, identifiers, version, permissions, optional packages, push, change
detection, regenerate and patch are all covered; `make spec-lint trace`.

Rules: additive proto only, `make gen`; a migration only if needed (next free number); no new deps, SPDX, no
TODO, never `expect(x).toBeNull()` inside waitFor. Do NOT run the full go-test or `make studio-check`.
Done-check once: `make gen-check go-lint`; `go test -race` on the changed packages; `cd studio && bun test
apps/web/src/features/shell-settings`; `bunx tsc -p apps/web --noEmit`; `make studio-lint`; lingui extract
(0 missing); `make spec-lint trace`. Commit (Conventional Commits header ≤72 chars) with the session
trailers. Do not push; no edits to docs/WORKLOG.md. Report ≤8 lines: commit, built, status, decisions, exit codes.
```
