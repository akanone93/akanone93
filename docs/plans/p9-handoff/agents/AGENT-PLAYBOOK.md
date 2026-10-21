<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Agent Playbook: multi-agent work at low token cost (binding, every project and phase)

## Session start
- Read the repo's working agreement (`AGENTS.md`, `CLAUDE.md` or the equivalent), its current work log or status file, and its active plan.
- Read nothing else until a task needs it.
- Re-read them after any context compaction. Never rely on a summary for rules or project state.

## Roles
- **Lead (this session):** plans, writes briefs, merges, reviews, and runs every quality gate and CI. It writes no feature code unless a merge or fix needs it.
- **Agents:** write feature code and its tests. Nothing else.

## Models and effort
- **Default code model:** Sonnet (latest; `claude-sonnet-5-5` today).
- **Haiku** (`claude-haiku-4-5`) is allowed for:
  - mechanical or well-specified work (renames, regenerations, test fixtures, small isolated features);
  - any task where the lead is confident it can verify the result fully.
- **Opus:** never for agent code. It is used only by the lead, for design, review and hard debugging.
- **Effort:** medium by default; high only for security, encryption, migrations or concurrency-heavy work; never max.
- **Definition:** a user-level agent definition (outside the repo) pins the model, the effort and a minimal tool list (Read, Edit, Write, Bash, Grep, Glob). No MCP, web or docs tools unless the task needs them.
- **The model doesn't lower the bar:** whatever the model, the lead must fully believe in the code before using it. If not, it gets fixed or redone.

## Work split
- **Order:** split the work into milestones and group them into waves. Within a wave, everything independent runs in parallel; dependent work goes in the next wave.
- **No gates between waves:** the next wave starts as soon as the previous wave's code is merged. Lint, gates, coverage, benchmarks, size, CI and long end-to-end runs happen once, centrally, by the lead at the end of the phase or batch.
- **Pre-allocated ranges:** assign error codes, limit keys, feature keys and IDs per agent up front, so merges don't collide.

## Agent rules (put them in every brief)
- Work in your own worktree and commit there. Never push.
- **Scope:**
  - features and their tests only;
  - run only your own tests;
  - no lint, gates, coverage, benchmarks, docs, changelog, work log or status changes.
- **Reading:** don't read beyond the files and snippets in the brief. Use `head`, ranges and grep, never full dumps.
- **On a conflict:** if a document conflicts with the task, report it; don't resolve it.
- **Report:** at most 8 lines (branch and commits, what was done, what wasn't, the seams it left, open questions).

## Briefs (the main token saver)
- **Shared prefix:** every brief starts with one identical block (repo rules, conventions, commit format and trailers, report format, agent rules), so repeated prefixes hit the prompt cache.
- **Agent-specific part, after the prefix:**
  - the exact files to touch;
  - the interfaces and signatures to use or implement, pasted in;
  - relevant schema and registry snippets;
  - the allocated ranges;
  - the decisions that apply;
  - the definition of done.
- **Gathering:** the lead collects this context once, with targeted greps, and reuses it across briefs. Agents shouldn't have to explore the repo.

## Lead rules
- **No polling:** wait for completion notifications. When asked for status, check with a one-line `git log` and `git status` per worktree.
- **Usage limits:** resume a stopped agent with SendMessage, never restart it. An agent that is done or stuck on gates gets told to wrap up.
- **Merging:**
  - hand-resolve source conflicts;
  - for generated and binary files, take one side, then regenerate with the repo's generators and golden-update flags;
  - never hand-format generated files.
- **Review:**
  - review every merged change;
  - raise to the user only incorrect behaviour or big technical debt, with a recommendation;
  - otherwise use the code as is.
- **Logs:** fetch CI logs into files (large tool output is saved to a file), then grep. Never pull full logs inline.
- **Output:** bound every command's output. Never dump large files.
- **Worktrees:** remove a worktree as soon as its branch is merged, and keep agent worktrees out of anything repo-wide checks scan.
- **Commits:** follow the repo's commit convention and hooks. Never bypass hooks.
- **Decisions:** record every user decision in the plan or decision log, with spec and ADR updates in the same change series.
- **Replies to the user:** short and in list form; ask only when a decision is genuinely theirs.

## Token hygiene
- Fewer, larger, well-briefed agents beat many cold-starting ones.
- Isolate noisy work (logs, searches, test-failure triage) in an agent that returns a short summary.
- Keep tool lists minimal; unused tool definitions cost tokens on every call.
- Don't re-read files you've already read or edited in the same session.
- Compacting is acceptable: state lives in the repo, so re-read the rules after compaction.
