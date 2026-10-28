<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Sub-agent rules (P11)

Binding for every brief. They extend [sub-agents.md](../../engineering/sub-agents.md) and
`AGENTS.md` §6. Token use is a first-class cost: the cheapest model, the fewest reads, the shortest
reports. Quality and security are guaranteed by the lead's review, not by the agent's tier.

## Models and effort (maintainer, 2026-10-27; as used in session 2)

| Work | Model |
|---|---|
| Mechanical: docs, registrations, translations, test-title fixes | **Haiku** or Sonnet |
| Feature slices, ports with goldens, UI with tests, follow-up fixes, docs sets | **Sonnet** |
| Security reviews, requirement statuses (evidence work), the five core parts (layout port, canvas core, arrow routing, editing core and undo, dev sessions across replicas) | **Opus** |

All agents are `general-purpose`, launched in the background. At most three at once.

## What an agent does

- Acts on the brief **at once**: no repository survey, no plan, no design alternatives, no reading of
  AGENTS.md or the handbook unless the brief names a section.
- Works **only in its worktree and branch**; never touches other worktrees; **never pushes**.
- Does exactly the listed items, in the listed priority; if budget runs out, commits what works and
  names the rest.
- Runs its **done-check once** at the end and reports exit codes, not impressions. It never runs
  `make studio-check` or the full `go-test` (the lead does; parallel full runs exhaust memory).
- Commits with Conventional Commits and the session trailers.
- **Reports in at most 6–8 lines**: done, not done, decisions it needs, done-check exit codes,
  commit SHA. Questions for the maintainer go in the report; the agent never decides them.

## What an agent never does

- Skip hooks, weaken or delete a test, lower a tolerance, budget, coverage floor or CSP rule,
  hand-edit generated files or goldens, add a dependency not named in the brief, add a limit or
  security default without saying so in the report, leave TODO/FIXME.

## Test rules (in every Studio brief)

- Never `expect(x).toBeNull()` on a DOM element inside `waitFor`: bun formats the element on each
  failed try and stalls about 25 s. Use `expect(x === null).toBe(true)`; the lint rule
  `studio/lint/no-waitfor-tobenull.grit` enforces it.
- Test titles carry the requirement IDs in brackets (`[STU-024]`); `make trace` reads only that form.
- Base UI selects: use the pattern of `features/action-graph/action-graph.test.tsx`.
- Real Monaco is mocked in component tests; canvas boot waits 5 s in unit tests, 15 s in e2e.
- Run `bun test` per feature directory; several directories in one call give spurious failures.
- Decide small technical points (safest option) and note them; stop only for a security default,
  a new dependency, a new limit or a proto change, and propose it.

## Token rules (in every brief)

- Read only the files named, and only the needed lines (`sed -n a,bp`, `grep -n`).
- Never print whole lockfiles, generated files, goldens, dependency sources or logs; pipe through
  `tail`/`grep`.
- No exploration; no re-reading the same file.
- **Tool-call cap** in every brief: 30–60 for Haiku/low, up to 100–120 only for large slices.

## Brief template

```
You are a short-lived sub-agent (AGENTS.md §6). Act on this brief at once; no survey, no plan.
Work ONLY in worktree <path> (branch <branch>). First `cd studio && bun install --frozen-lockfile`.
Never push.
Task: <step id, spec IDs, the exact files/lines to read, the pattern file to copy>.
Do: 1. … 2. … (priority order; what to do if budget runs out).
Rules: SPDX headers; no TODO/FIXME; no new dependencies; never weaken tests/tolerances/CSP;
generated code only via `make gen`; limits from schema/limits.json.
Tokens: sed -n ranges, grep, tail. At most <N> tool calls.
Done-check once at the end: <commands> (exit codes).
Commit `<type>(<scope>): … (<step>)` ending with the two trailer lines.
Report in at most 6 lines: done, not done, done-check exit codes, commit SHA.
```

The lead never reads an agent's transcript; the report is the interface. The lead verifies every
claim on the merged tree.
