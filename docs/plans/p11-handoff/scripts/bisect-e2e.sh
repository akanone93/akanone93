#!/bin/bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
# `git bisect run` helper for an e2e regression, run inside a dedicated worktree:
#   git worktree add --detach .claude/worktrees/bisect HEAD
#   cd .claude/worktrees/bisect && git bisect start --first-parent <bad> <good>
#   git bisect run docs/plans/p11-handoff/scripts/bisect-e2e.sh <spec-filter>
# Exit 125 skips a commit that does not build.
cd "$(git rev-parse --show-toplevel)" || exit 125
(cd studio && bun install --frozen-lockfile >/dev/null 2>&1)
make studio-build >/dev/null 2>&1 || exit 125
out=$(cd studio && timeout 600 bunx playwright test --config e2e/playwright.config.ts "${1:-s3-flow}" 2>&1)
grep -q ' passed' <<<"$out" && ! grep -q ' failed' <<<"$out"
