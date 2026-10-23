<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E12e — two coverage floors and one test race

1. **Coverage floors (`QA-001`, plan §8 A6: 85 % for security-relevant packages):** `backend/internal/approval` is at 80.2 % and `backend/internal/function` at 81.3 %. Add meaningful tests (failure and abuse paths first: refused decisions, expired and invalidated requests, policy matching edge cases, signing of records; function deploy/enable refusals) until each is ≥ 86 %. Measure with `cd backend && go test -count=1 -coverprofile=/tmp/c.out ./internal/approval/ && go tool cover -func=/tmp/c.out | tail -1` (same for function). Never lower a floor in `coverage.json`.
2. **Race in `packages/plux_flutter/test/control/control_runtime_test.dart`**, test "on launch and on resume, and every five minutes in the foreground only; a 304 changes nothing [SYN-060]": under load the first timer-fired check can still be running when the test pauses and resumes; the resume's check then joins it (`ControlPoller.checkNow` joins a running check by design), so no third request is ever made and `settle(… controlAsked.length == 3)` times out. Fix the test so it waits until the first check has fully finished before pausing (add the smallest test-visible signal if none exists — e.g. the poller's in-flight future — without changing the join behaviour). Prove it: run that file 10 times in a row while `yes > /dev/null &` loads two cores (kill them after).
Done when: both packages are ≥ 86 % and `cd packages/plux_flutter && flutter test test/control && dart analyze lib test` pass.
