<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# The lead's job

The lead is the main agent of the session. Sub-agents build; the lead owns quality, security and
the branch. Base rules: [sub-agents.md](../../engineering/sub-agents.md); this file adds what P11
taught (sessions 1 and 2).

## The maintainer's standing preferences

- **Token efficiency is a priority.** The lead never reads agent transcripts; reports are the
  interface. Read only the lines needed.
- **The lead enforces the quality and security gate.** Agents' "done" is a claim; verify on the
  merged tree.
- **Decide technical points yourself** (maintainer, 2026-10-28) — limits proposed by an agent,
  small design choices, test approaches, tightenings — record each in [decisions.md](decisions.md)
  marked "(lead, delegated)". Ask the maintainer only for decisions that need exactly their input:
  product scope, loosening a security control, anything outward-facing (opening a PR, publishing).
- **Short replies.** Status in one short sentence when asked; progress in percent (total and per
  wave) when asked.
- **One full gate and one push per milestone**, not per slice (maintainer, 2026-10-28). The stop
  hook nags about unpushed commits: answer it in one line and keep to the milestone rule.
- **"Usable at the end of the phase":** the maintainer expects to run the whole project and use it
  at the phase exit (later phases excluded). Look for missing user-facing flows, not only spec rows
  (session 2 found the org/app creation screens and the S8.1 web part missing this way).
- **"Stop" means stop:** stop every agent (TaskStop) and process, push nothing, wait.
- **After the P11 exit:** report, then wait for the go-ahead and notes before analysing or planning
  the cross-phase audit phase ([next.md](next.md) step 7).

## Responsibilities

- **Brief and launch.** `git worktree add -q .claude/worktrees/<slice> -b p11/<slice> feat/phase-11`
  (`.claude/worktrees` is in `.git/info/exclude`), a self-contained brief
  ([agent-rules.md](agent-rules.md)), the cheapest model that can do it, background, **at most three
  agents at once** (15 GB of memory: four parallel test runs ran out).
- **Review every diff yourself** before merging: authz and tenancy (another organisation must get
  not found; new `app_id` tables need `plux_tenant_policy` and `plux_app_policy`; personal data goes
  into `backend/internal/dsr/datamap.go` + `make gen`), CSP and Trusted Types, secrets, layering
  (L-4: the builder, fnrunner, egress and render worker hold no credentials), hard-coded limits
  (LIM-001), dependencies, outbound connections (a new internal client is registered in
  `backend/internal/httpx/completeness_test.go`), tests that test behaviour, no TODO.
- **Merge and resolve conflicts yourself** (`git merge --no-ff -q p11/<slice> -m "chore: merge …"`;
  the commit hook requires a Conventional Commits header and the trailers), fix small integration
  defects yourself, remove the worktree and branch after the merge.
- **Follow-ups, not resumes.** A partial result gets a short follow-up brief; an agent stopped by a
  usage limit gets one "continue" message after the reset (SendMessage), not a relaunch.
- **Migrations:** give each parallel agent its own number in the brief (collisions happened).
- **Keep documents and code in agreement** in the same commit; work log at the end of each block.

## The gate and the push

`docs/plans/p11-handoff/scripts/gate.sh` (gen-check, go-lint, studio-check, studio-e2e; pushes only
when all pass; extra arguments are extra refs to push). Run it in the background on an idle
machine. The pre-push hook then runs the full `make go-test` (about 30 minutes; the devsession
package alone takes about 2 minutes); under load it times out, so never push while agents run.
Run `go clean -cache` first when the disk is above about 85 %.

## Recipes

- **`.po` conflicts** (almost every Studio merge): take ours, re-extract, refill from both sides:
  ```sh
  for l in en de ar; do git checkout --ours studio/apps/web/src/locales/$l/messages.po; done
  (cd studio/apps/web && bunx lingui extract)
  python3 docs/plans/p11-handoff/scripts/pofill.py HEAD <merged-branch>
  (cd studio/apps/web && bunx lingui extract)   # de and ar must show 0 missing
  ```
  Caveat: "ours" keeps HEAD's English for a message the branch reworded. Rebuild `en` when tests
  fail on changed wording: delete `en/messages.po`, extract, and take the branch's de/ar for the
  changed ids (this broke two suites once).
- **Additive code conflicts:** `scripts/union.py <file>` keeps both sides; it can split a function
  body, so check with `bunx tsc -p apps/web --noEmit`.
- **Regression hunt:** `git bisect run scripts/bisect-e2e.sh <spec>` in a detached worktree.
- **Benchmarks:** `scripts/quiet-bench.sh` waits for a quiet machine; budgets miss under load.
- **Canvas worker:** only cloneable data goes to a worker; a function in a `postMessage` payload
  fails silently (`DataCloneError`, the canvas never starts).
- **Biome:** `useLiteralKeys` infos are not errors; `make studio-lint` is the gate.
- **Generated Studio TypeScript (`*.gen.ts`) must select go-lint** in `ci/affected.json`.
- **Disk:** about 40 GB allowance; `go clean -cache`, remove `studio/test-results`; deleting `/tmp`
  build folders is refused by auto mode.
- **`pkill -f` can kill your own shell:** use `pgrep -af` and kill by PID.
- **Background commands:** a commit in a background command holds `.git/index.lock` while its hooks
  run; do not start another git write meanwhile.
