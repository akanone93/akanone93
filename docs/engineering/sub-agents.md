<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Sub-agents: how the lead runs them

The rules behind AGENTS.md §6 ("direct and short-lived"), learnt in P10. The goals: code to the
repository's standard, with the fewest tokens.

## Roles

- **Lead:** reads the plan once, writes the briefs, creates the worktrees, reviews every diff,
  fixes integration defects, merges, runs the gates, pushes, and asks the maintainer about
  AGENTS §7 items. The lead owns quality; sub-agents own one slice.
- **Sub-agent:** builds exactly one brief, runs its done-check once, commits and reports. It never
  investigates, plans, fixes another slice or touches CI.

## Before launching

1. **The lead creates the worktree:** `git worktree add -b p<N>/<slice> .claude/worktrees/<slice> <phase-branch>`.
   Never use automatic worktree isolation: it cuts from `main`, and moving it later costs a whole
   agent.
2. **Fix shared seams first.** Write the interfaces that parallel slices share (proto messages,
   Dart interfaces, wire formats) and commit them before the fan-out, so agents don't invent
   conflicting ones.
3. **Settle §7 questions first:** dependencies, limits, security wording. Ask the maintainer in
   one batch.

## The brief: pre-digested, no survey

The lead reads once and the agent reads nothing extra. Every brief has:

- **The worktree path and branch,** then "Act at once: no survey, no plan, do not read AGENTS.md
  or standards."
- **READ:** the exact files, with line ranges where the file is long, plus the exact proto or spec
  text the slice depends on.
- **PATTERNS:** one existing file to copy per concern (service, handler, migration, test harness).
- **BUILD:** the design is decided, not open: names, signatures, error codes, limit keys and
  behaviour.
- **RULES (inline, five lines at most):** SPDX header, no TODO, no globals, limits from the
  registry, generated code only via `make gen`, layering rules that apply.
- **TESTS:** the cases to cover and the requirement IDs to name in them.
- **DONE-CHECK:** one command line, run once.
- **Commit message** (header of 72 characters at most, with the attribution trailers), "Do not
  push", and a report of eight lines at most.

## Sizing and models

- **One slice is one package or file group,** about 30 tool calls. Split anything bigger; a large
  brief means an agent that wanders.
- **Haiku** for mechanical work (templates, registrations, docs). **Sonnet** by default. **Opus**
  only for security, cryptography, migrations or concurrency-critical code.
- **Run independent slices in parallel,** no more than you can review at once, and never two
  slices editing the same file.
- **Don't resume a long agent.** Finish a small remainder yourself, or launch a short follow-up
  brief.

## Review and merge (lead)

1. **Read the diff:** security paths (tokens, keys, verification, authorisation), layering,
   globals, hard-coded limits, test quality (real behaviour, not coverage padding).
2. **Merge into the phase branch** and run the touched packages' analyze and tests. Resolve
   conflicts yourself.
3. **Fix integration defects yourself** when they are small. Fire a new agent only to *build*,
   never to *investigate*.
4. **Reconcile agent deviations** with the documents. A real divergence goes to the maintainer
   (AGENTS §3).
5. **Commit and push after each merged slice.** Push when no heavy test run is in progress:
   running the pre-push hook under load produces false failures.

## Token rules

- The lead's reads are excerpts (`grep -n`, `sed -n a,bp`), never whole large files or logs.
  Filter command output (`tail`, `grep`).
- Never read an agent's transcript; the eight-line report is the interface.
- Briefs quote what the agent needs instead of telling it to find it.
- No long gates per slice. `make check`, CI and the benchmarks run once, at the end.
- Status replies to the maintainer are short: a table or one sentence.

## End of phase

`make spec-lint` and `make trace` with statuses set, coverage floors met (add tests, never lower a
floor), `make check`, `docs/WORKLOG.md`, then one PR. CI runs on the PR; don't dispatch it by hand.
