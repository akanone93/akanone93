<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Brief: app usage, last publisher and the per-plugin usage view

Lead setup: `git worktree add -q .claude/worktrees/g-usage -b p11/g-usage feat/phase-11`.
Model: Sonnet (general-purpose agent, background). Server work with tenancy: review it closely.

```
You work in the git worktree /home/user/akanone93/.claude/worktrees/g-usage (branch p11/g-usage). Run every
command there; first `cd studio && bun install --frozen-lockfile`. Read AGENTS.md §5–§7, backend/AGENTS.md,
studio/AGENTS.md, docs/requirements.md rows STU-022, STU-025, LIM-005 and spec §30.4, and
studio/apps/web/src/features/apps. Act at once: no plan. Decide small technical points yourself (safest option);
stop only for a security default, new dependency or new limit.

1. STU-022: AppSummary (AppProjectService.ListAppSummaries) has no publisher of the last release. Add an additive
   field (the Actor message used elsewhere, and the time if missing), filled server-side from the release
   history, and show it on the app card.
2. STU-025 / LIM-005: the summary reports usage only for app.plugins. Measure current usage at app scope for every
   limit with app scope in schema/limits.json that has a meaningful current value (pages, assets and asset bytes,
   functions, release size, translations…; skip rate and per-request limits and list them with the reason) in the
   summary and in ListAppLimits, with one batch query per page of summaries (no N+1) under the existing RLS.
   Show them on the card meters and in the app settings usage table.
3. LIM-005 per plugin: a per-plugin usage view (limits with plugin scope): extend a plugin read or add an
   additive RPC (authorization plugin.read like its neighbours; another organisation gets not found), and a
   Studio view in the plugin settings or the plugin detail card.
Integration tests in Go (test database, -race) including tenancy; component tests with [ID] in the titles. Set
STU-022, STU-025, LIM-005 to DONE in docs/requirements.md only if fully covered; `make spec-lint trace`.

Rules: additive proto only, `make gen`; a migration only if needed, number 0066; no new deps, SPDX, no TODO,
never `expect(x).toBeNull()` inside waitFor. Do NOT run the full go-test or `make studio-check`. Done-check once:
`make gen-check go-lint`; `go test -race` on the changed packages; `cd studio && bun test` on the changed
directories separately; `bunx tsc -p apps/web --noEmit`; `make studio-lint`; lingui extract (0 missing);
`make spec-lint trace`. Commit (Conventional Commits header ≤72 chars) with the session trailers. Do not push; no
edits to docs/WORKLOG.md. Report ≤8 lines: commit, built, statuses, skipped limits, exit codes.
```
