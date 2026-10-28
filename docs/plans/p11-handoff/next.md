<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Next steps, in order (session 3)

Models per [agent-rules.md](agent-rules.md); at most three agents at once. Briefs are in
[briefs/](briefs/) and can be launched unchanged (create the worktree first, as each brief says).

1. **Finish the S8.1 wizard (Sonnet).** Launch [briefs/s81-wizard-finish.md](briefs/s81-wizard-finish.md)
   on the pushed work-in-progress branch `p11/g-wizard`. STU-015, STU-024, GEN-004.
2. **App usage and publisher (Sonnet), in parallel.** Launch [briefs/s81-usage.md](briefs/s81-usage.md)
   from `feat/phase-11`. STU-022, STU-025, LIM-005 (server measurement, an additive proto field, the
   per-plugin view).
3. **Lead: review and merge both** (security, tenancy, limits; `.po` recipe in [lead.md](lead.md)),
   then the full gate and one push: `docs/plans/p11-handoff/scripts/gate.sh` on an idle machine.
4. **Lead: `make check`** (whole repository) once, in the background; fix what it finds.
5. **Ask the maintainer** to open the pull request `feat/phase-11` → `main` for the one CI run of
   M-6 (an outward-facing action: always ask first). The jobs that only CI can run: the Studio and
   render-worker images (`studio-image-repro`, DEP-001, DEP-002, STU-100), `zap-studio` (QA-008),
   `studio-bench` (QA-007, STU-045 drag budgets on a hosted runner), and the stack jobs. Drive the
   PR to green per the PR rules of the environment; a failure that CI alone shows is fixed on the
   branch.
6. **Lead: the S9.7 exit.** Update statuses from the CI evidence (`make spec-lint trace`), the work
   log, the exit report to the maintainer (format of `AGENTS.md` §9), including what stays with the
   maintainer: the screen-reader pass (STU-011), the native review of `de`/`ar` (STU-012, L-11),
   the reference-laptop benchmark (STU-120, NFR-031, M-4), the six legal reviews (O-7; list in
   [open.md](open.md)).
7. **Stop and wait.** After the exit report, do not start, analyse or plan the cross-phase audit
   phase until the maintainer gives the go-ahead with notes (maintainer, 2026-10-28). The audit
   covers the requirements of every phase, P0–P11 (about 75 earlier rows are `WIP`), aims at
   everything working and secure with acceptable performance, and is not to be made so deep that
   it takes days; it gets its own plan and milestones.
