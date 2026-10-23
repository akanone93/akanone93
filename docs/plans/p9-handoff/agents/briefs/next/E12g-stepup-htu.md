<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E12g — the CLI step-up fails its DPoP `htu` check (`SEC-103`, `SEC-092`)

CI run 39721074609: `make bench-sync` (`backend/internal/server/sync_bench_integration_test.go:97`) and `make restore-drill` fail with `plux publish: permission_denied: PLX-6200: this action needs a step-up with a security key` then `step-up: PLX-6010: dpop proof failed check "htu"`. The CLI browser step-up (`backend/internal/auth/stepup_cli.go`, `backend/cmd/plux` step-up client, `backend/internal/api/stepup.go`) builds a DPoP proof whose `htu` does not match what the server checks — likely the server compares against its `publicBaseURL` while the CLI targets the URL it was given (a port-forward or httptest address), or a path/query mismatch. Find the exact mismatch, fix it on the side that is wrong per RFC 9449 §4.3 and the existing DPoP checks (`backend/internal/dpop`), never by loosening the check. Then find why `plux publish` in those two tests needs a step-up at all (which profile/permission now asks for it) and make the tests do the step-up the way a real user would (the test harness has a fake WebAuthn path — grep `WithStepUp`), not by disabling it. Add a regression test of the htu case.
Done when: `cd backend && go test -count=1 ./internal/dpop/... ./internal/auth/... ./internal/api/... ./cmd/plux/` and `PLUX_BENCH_SYNC=1 go test -count=1 -run TestSyncOnSlowNetwork ./internal/server` (check `mk/bench.mk` for the real env name) pass.
