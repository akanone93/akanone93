#!/bin/bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
# The lead's milestone gate: gen-check, go-lint, studio-check, studio-e2e; pushes the current
# branch only when all four pass (the pre-push hook then runs the full go-test). Logs go to $LOG_DIR.
# Run on an idle machine (no agents), after `go clean -cache` when the disk is low.
set -u
cd "$(git rev-parse --show-toplevel)"
LOG_DIR=${LOG_DIR:-$(mktemp -d)}
branch=$(git branch --show-current)
make gen-check > "$LOG_DIR/gen.log" 2>&1; g=$?; echo "gen-check $g"
make go-lint > "$LOG_DIR/lint.log" 2>&1; l=$?; echo "go-lint $l"
make studio-check > "$LOG_DIR/studio.log" 2>&1; c=$?; echo "studio-check $c"
grep -E '^ *[0-9]+ (pass|fail)' "$LOG_DIR/studio.log"; grep -E '^\(fail\)' "$LOG_DIR/studio.log" | head
make studio-e2e > "$LOG_DIR/e2e.log" 2>&1; e=$?; echo "e2e $e"; grep -E 'passed|failed|flaky' "$LOG_DIR/e2e.log" | tail -3
if [ "$g$l$c$e" = 0000 ]; then
  go clean -cache
  git push -u origin "$branch" "$@" > "$LOG_DIR/push.log" 2>&1; echo "push $?"; tail -5 "$LOG_DIR/push.log"
  grep -E '^(--- FAIL|FAIL)' "$LOG_DIR/push.log" | head
else
  echo "NOT PUSHED (logs in $LOG_DIR)"
fi
