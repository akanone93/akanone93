<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Brief: finish the S8.1 new-app wizard, onboarding and shell settings

Lead setup: `git fetch origin p11/g-wizard && git worktree add -q .claude/worktrees/g-wizard p11/g-wizard`.
Model: Sonnet (general-purpose agent, background).

```
You work in the git worktree /home/user/akanone93/.claude/worktrees/g-wizard (branch p11/g-wizard). Run every
command there; first `cd studio && bun install --frozen-lockfile`. Read AGENTS.md §5–§6, studio/AGENTS.md,
docs/requirements.md rows STU-015, STU-024 and GEN-004, docs/adr/0102-plux-archive.md, and the Porcelain design
guidance (plux-studio-design skill under .claude/skills). Act at once: no plan. Decide small technical points
yourself (safest option) and note them; stop only for a security default, new dependency, new limit or proto change.

State: the branch has an unfinished commit ("feat(studio): unfinished new-app wizard, onboarding (S8.1)") with
features/new-app/ (wizard: blank, template, .plux import through the import job RPC, locales, theme, profile),
features/onboarding/ (checklist, sample apps) and features/shell-settings/ (no tests yet). Nothing was verified.
1. Merge feat/phase-11 into this branch (`git merge origin/feat/phase-11`): conflicts are expected in
   features/home/ (feat/phase-11 now embeds the apps grid AppsArea in home-page.tsx and keeps a create dialog;
   the wizard replaces the create dialog: keep the grid, route "New app" to the wizard) and the .po catalogues
   (recipe in docs/plans/p11-handoff/lead.md). Then `bunx tsc -p apps/web --noEmit` clean.
2. Finish what is missing so STU-024, STU-015 and GEN-004 hold completely, using existing RPCs only
   (AppProjectService: ImportApp, ListSampleApps, ImportSampleApp, GetShellStatus, SetShellSettings,
   GenerateProject, GetProjectJob, DownloadProject). Shell settings is reached from the app settings page
   (route .../apps/$app/settings, features/apps/) as .../settings/shell.
3. Tests: component tests for each requirement with the ID in brackets in the title ([STU-024]), including the
   server's refusals and the archive limits (limitDefault of archive.bytes, archive.entries,
   archive.expandedBytes); shell settings tests. Lingui en/de/ar complete.
4. Set STU-015, STU-024, GEN-004 to DONE in docs/requirements.md only when fully covered; `make spec-lint trace`.

Rules: no new deps, SPDX, no TODO, never `expect(x).toBeNull()` inside waitFor (lint rule), Base UI selects in
tests use the pattern of action-graph.test.tsx. Do NOT run `make studio-check`. Done-check once:
`cd studio && bun test` on features/new-app, features/onboarding, features/shell-settings, features/home,
features/apps separately; `bunx tsc -p apps/web --noEmit`; `make studio-lint`; lingui extract (0 missing);
`make spec-lint trace`. Commit (Conventional Commits header ≤72 chars) with the session trailers. Do not push;
no edits to docs/WORKLOG.md. Report ≤8 lines: commit, built, statuses, decisions, exit codes.
```
