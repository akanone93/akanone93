<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# The lead's job

The lead is the main agent of the session. Sub-agents build; the lead owns quality, security and
the branch. Base rules: [sub-agents.md](../../engineering/sub-agents.md); this file adds what P11
taught.

## Responsibilities

- **Brief and launch.** Create each worktree (`git worktree add -q .claude/worktrees/<slice> -b p11/<slice> feat/phase-11`),
  write a self-contained brief ([agent-rules.md](agent-rules.md)), pick the cheapest model that can
  do it, launch in the background, at most 4–6 at once.
- **Review every diff yourself** before merging: security (authz, tenancy — e.g. cross-organisation
  references the FK does not catch, CSP/Trusted Types, secrets), layering, hard-coded limits
  (LIM-001: limits come from `schema/limits.json`), dependencies (only the allowlist or the
  decisions table, exact pins), tests that test behaviour, no TODO/FIXME. Agents' "done" is a claim;
  verify the gates on the merged tree.
- **Merge and resolve conflicts yourself** (`git merge --no-ff --no-edit p11/<slice>`), run the
  touched gates, fix small integration defects yourself, commit and **push after every merged
  slice**. Never rewrite pushed history.
- **Follow-ups, not resumes.** A partial or wrong result gets a new short brief with exactly the
  remainder and the defects found; never investigate through an agent.
- **Ask the maintainer** for every AGENTS.md §7 item: new limits or limit defaults, dependencies,
  security controls (CSP, permissions), gates, any deviation from a document (§3: present the
  options with a recommendation). Record answers in [decisions.md](decisions.md) and the work log.
- **Keep documents and code in agreement** in the same commit (ADR revision, handbook, reference).
- **Work log** at the end of each work block; keep ten notes, archive the rest unchanged.
- **Status replies are one short sentence** unless the maintainer asks for more.

## Recipes

- **Translation catalogues (`.po`) conflict on almost every Studio merge.** Union-merge, re-extract,
  then refill translations the union dropped:
  ```sh
  S=<scratch dir>; for f in studio/apps/web/src/locales/*/messages.po; do
    git show :2:$f > $S/o.po; git show :1:$f > $S/b.po; git show :3:$f > $S/t.po
    git merge-file -p --union $S/o.po $S/b.po $S/t.po > $f; done
  (cd studio && apps/web/node_modules/.bin/lingui extract --clean)
  python3 docs/plans/p11-handoff/scripts/pofill.py HEAD MERGE_HEAD
  (cd studio && apps/web/node_modules/.bin/lingui compile --strict; rm -f apps/web/src/locales/*/messages.js)
  ```
  `make studio-i18n-check` compares with the index: stage the catalogues before running it.
- **Shared Studio seams conflict often:** `features/overlay/use-overlay-host.ts` (`OverlayId` — take
  the union), `features/panels/right-dock.tsx` and `features/plugin-canvas/plugin-canvas-page.tsx`
  (keep both sides' props/JSX; check every JSX element is closed), `apps/web/src/test-upstream.ts`
  (the RPC stubs; the harness re-exports it).
- **Layout goldens:** after merging layout work run `make studio-conformance-update` and confirm
  `git status -- test/layout-conformance/goldens` is empty (regenerated goldens identical).
- **Biome:** `useLiteralKeys` *infos* are not errors; judge by `bunx biome ci` exit code. Keep
  `buildNode` (layout `scene.ts`) under cognitive complexity 15.
- **Dart:** `make dart-analyze` treats infos as fatal; in a fresh worktree run `flutter pub get` at
  the root first or `plux_svgc` reports false errors.
- **Pinned transforms** in `studio/apps/web/vite.config.ts` (sonner, Monaco style nonce) fail the
  build on any other version: an upgrade must review them.
- **Heavy runs:** run `make studio-check`/`studio-e2e` in the background (`run_in_background`) and
  push when no heavy run is in progress. Benchmarks only on an idle machine (`uptime`).
- **Disk** is limited (~40 GB allowance): remove merged worktrees
  (`git worktree remove --force`), and `go clean -cache` when above ~90 %.
- **Session limits / worker restarts:** agents stop with "session limit"; after the reset send each
  one a short "continue" message (SendMessage) instead of relaunching. After a worker restart,
  restart Postgres and Valkey.
- **`pkill -f` kills your own shell**: use `pgrep -af` and kill by PID.
