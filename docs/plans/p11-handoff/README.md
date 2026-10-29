<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# P11 hand-off (2026-10-28, end of session 2)

Everything a new session needs to continue P11 (Plux Studio) exactly where session 2 stopped, and
to work the same way. Read this file, then the files it names, in order. Nothing here overrides
`AGENTS.md`, the spec or the ADRs; it records state and the working method. The package follows
[handoff-procedure.md](../handoff-procedure.md).

## Where we are

| | |
|---|---|
| Phase | P11, [plans/p11.md](../p11.md), on branch `feat/phase-11` (pushed; head in [state.md](state.md)) |
| Total | about **95 %** of P11 |
| Waves 1–11 and the gap-fix wave | done, merged and pushed |
| Phase exit S9.7 | statuses set (82 DONE, 15 WIP, 2 withdrawn of 99 P11 rows; `make spec-lint trace` 0); the S8.1 web part was found missing during the exit and is being finished |
| Unfinished | S8.1 web: new-app wizard, onboarding, shell settings (branch `p11/g-wizard`, work in progress); app usage and last publisher (not started) |
| Then | the PR with one CI run (needs the maintainer's yes), the exit report, then **wait** for the maintainer's go-ahead on the cross-phase audit phase |

## Start the next session

1. Follow `AGENTS.md` §2 (re-read it, `docs/WORKLOG.md`, `git status`); `git checkout feat/phase-11`.
2. Read, in order: [lead.md](lead.md) (your job and the maintainer's preferences),
   [agent-rules.md](agent-rules.md) (how agents run), [state.md](state.md), [open.md](open.md),
   [decisions.md](decisions.md), [verification.md](verification.md).
3. Services: `service postgresql start`; `redis-server --save "" --daemonize yes`
   (`PLUX_TEST_DATABASE_URL` is set by the environment).
4. Continue with [next.md](next.md).

Starting prompt for the new session:

> Continue P11 from `docs/plans/p11-handoff/README.md` on branch `feat/phase-11`, then follow
> next.md. You are the lead: agents per agent-rules.md (at most 3), you review and merge
> everything, one full gate and one push per milestone, short status replies.
