<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# State at hand-off (2026-10-28, end of session 2)

## Branches

| Branch | State |
|---|---|
| `feat/phase-11` | All merged work, pushed with this package. The last commits were **not** through the full gate (stopped at the hand-off); run it first ([verification.md](verification.md)). |
| `p11/g-wizard` | **Work in progress, pushed, not merged, not reviewed.** S8.1 web part 2: new-app wizard (`studio/apps/web/src/features/new-app/`: blank, template, `.plux` import, locales, theme, profile), onboarding checklist and sample apps (`features/onboarding/`), shell settings (`features/shell-settings/`), route and fixture changes (`router.tsx`, `test-upstream.ts`, `test-projects.ts`), catalogues. Its agent was stopped by the maintainer: shell settings has no tests, no done-check ran. Based on `feat/phase-11` before the apps grid merge: expect conflicts in `features/home/` and the `.po` catalogues. Finish with [briefs/s81-wizard-finish.md](briefs/s81-wizard-finish.md). |
| `p11/s13-gap`, `p11/s13-tiles` (remote) | Old canvas experiments, deliberately not merged. Leave them. |
| `handoff/p11-session2` (remote) | The session 1 package; already merged. |

The usage slice (STU-022 publisher, STU-025 usage, LIM-005 per-plugin) was started and dropped (one
SQL query); restart it from [briefs/s81-usage.md](briefs/s81-usage.md).

## Done in session 2 (merged on `feat/phase-11`)

- Waves 7–10 (S4.5–S8.10) and the S5 exit gate: Rust editor and LSP client, last canvas ports and
  the golden matrix, clipboard, library, devices and frames, device loop, action bar, release
  requests and the render worker, publish, releases, approvals, experiments, the S8 areas.
- Gap-fix wave (every gap of `open.md`): function version fields, session replay (ADR-0101),
  clipboard copy-over, deep plugin duplicate, undo for restore and key extraction, full experiments,
  ordered funnels, host and uploaded fonts, MenuAnchor and NavigationRail, app navigation, mock
  bindings, publish rollout form and evidence, `plux test --report`, the `no-waitfor-tobenull` lint
  rule, create organisation and app dialogs.
- Wave 11: S9.1 performance gates in CI, S9.2 axe sweep (75 routes × light, dark, `ar`) and keyboard
  flow, S9.3 catalogues complete, S9.4 threat model and ASVS L2 evidence for Studio
  (`worker-src 'self'`), S9.5 reproducible build, release jobs, render worker client, Compose and
  Helm, S9.6 guides, BFF runbook, run-locally guide.
- Exit work: P11 statuses; apps grid, card actions and app settings (STU-020, STU-021, STU-023);
  plugin detail card (STU-031); template page-state parameters (STU-082).

Migrations used up to **0065** (`0065_app_host_fonts.sql`). Next free: **0066**.
