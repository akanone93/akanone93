<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Session hand-off procedure

How a lead session packages its work so the next session continues exactly where it stopped and
works the same way. Approved by the maintainer on 2026-10-28 (end of P11 session 2). Use it for
every hand-off; the result lives in `docs/plans/<phase>-handoff/`.

Keep it quick: the whole procedure is a work block of its own, but the analysis is a short,
targeted pass (minutes, not half an hour).

## Phase 1 — Analysis (short)

- Remaining work of the phase: the `WIP` rows of the phase in `docs/requirements.md` with one-line
  reasons, the gaps in `open.md`, the next slices of the plan.
- Running or stopped agents: for each worktree, what is committed, what is uncommitted, whether it
  builds; decide keep (commit as work in progress on its own branch) or drop (the slice is
  restarted from its brief).
- Documents against code: work log, `open.md`, `decisions.md`, `state.md`, requirement statuses.
- Things that live outside the repository (scratch scripts, local branches, worktrees, notes) and
  would be lost.
- Machine state: disk, services, unpushed commits.

## Phase 2 — Save and push everything

- Commit kept work in progress on its own branch (Conventional Commits; the header says it is
  unfinished; hooks are never skipped — fix what they reject). Never merge it into the phase branch.
- Move every helper that matters into `docs/plans/<phase>-handoff/scripts/` (SPDX header, no
  machine-specific paths).
- Remove merged worktrees and branches with no commits of their own.
- Run the full gate on the phase branch (`scripts/gate.sh`: gen-check, go-lint, studio-check,
  studio-e2e) on an idle machine, then push the phase branch and the work-in-progress branches in
  one push (one pre-push run of the full Go tests).

## Phase 3 — Documents

- `docs/WORKLOG.md`: current focus, a new hand-off note (newest first, keep ten, archive the rest
  unchanged).
- `open.md`: only what is really open; `decisions.md`: every maintainer and lead decision of the
  session; requirement statuses match the evidence (`make spec-lint trace`).

## Phase 4 — Hand-off notes (`docs/plans/<phase>-handoff/`)

- `README.md`: where we are (percent, branches, heads), how to start, the starting prompt.
- `state.md`: what is merged, pushed, on which branch, and what is half done.
- `next.md`: the exact next actions in order, each with its model tier and a link to its brief.
- `briefs/`: a ready-to-launch brief for each unfinished slice, written so the next lead can
  launch it unchanged.

## Phase 5 — Operating notes

- `lead.md`: how the lead works (agents, review, merge recipes, gate, pushes, reporting).
- `agent-rules.md`: model tiers, brief template, test and token rules.
- `verification.md`: the last gate results and what was not run.
- The maintainer's standing preferences (in `lead.md`).

## Phase 6 — Final check and push

- Read the package cold, as the next session would: can it start from `README.md` alone?
- Commit, push, and report to the maintainer with the starting prompt for the next session.
