<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# P11 hand-off (2026-10-27, end of session 1)

Everything a new session needs to continue P11 (Plux Studio) exactly where the first session
stopped. Read this file, then the files it names, in order. Nothing here overrides `AGENTS.md`,
the spec or the ADRs; it records state and the working method.

## Where we are

| | |
|---|---|
| Phase | P11, [plans/p11.md](../p11.md), on branch `feat/phase-11` (pushed, head `daa1d1c8`) |
| This package | branch `handoff/p11-session2` = `feat/phase-11` + this folder + the work log note |
| Total | about **62 %** of P11 |
| Waves 1–6 (S0–S4.4) | done and merged |
| Wave 7 (S5 + S4.5, S4.6) | about **80 %**: S4.5, S5.1, S5.2, S5.3, S5.5, S5.6 merged; S5.4 server merged, its editor unfinished; S4.6 unfinished; S5 exit gate not run |
| Waves 8–11 | not started |

## Start the next session

1. Follow `AGENTS.md` §2 (re-read it, the work log, `git status`).
2. Read, in order: [lead.md](lead.md) (your job), [agent-rules.md](agent-rules.md) (how agents run),
   [state.md](state.md) (what is merged and what is half done), [open.md](open.md) (gaps and
   questions), [decisions.md](decisions.md) (maintainer decisions), [verification.md](verification.md).
3. `git checkout feat/phase-11` (work continues there; this branch only carries the package —
   merge it into `feat/phase-11` first so the documents travel with the code).
4. Restart services if needed: `service postgresql start`; `redis-server --save "" --daemonize yes`.
5. Continue with [next.md](next.md): the three unfinished branches first, then the S5 exit gate,
   then wave 8.

Starting prompt for the new session:

> Continue P11 from `docs/plans/p11-handoff/README.md` on branch `handoff/p11-session2`: merge it
> into `feat/phase-11`, then follow next.md. You are the lead: agents per agent-rules.md, you review
> and merge everything, short status replies.
