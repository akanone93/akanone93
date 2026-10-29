#!/bin/bash
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
# Waits up to 10 minutes for the 1-minute load average to drop below 2.5, then runs a bench
# target (default studio-bench-canvas): the canvas budgets are only meaningful on a quiet machine.
cd "$(git rev-parse --show-toplevel)" || exit 1
for _ in $(seq 120); do
  l=$(cut -d' ' -f1 /proc/loadavg)
  if awk "BEGIN{exit !($l < 2.5)}"; then break; fi
  sleep 5
done
echo "start load $(cut -d' ' -f1-3 /proc/loadavg)"
make "${1:-studio-bench-canvas}"; rc=$?
echo "exit $rc, end load $(cut -d' ' -f1-3 /proc/loadavg)"
exit $rc
