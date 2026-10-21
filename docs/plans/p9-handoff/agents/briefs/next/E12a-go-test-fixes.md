<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E12a — make `make go-test` green on `feat/phase-9`

Several slices merged in parallel; the full Go suite now fails in six packages (log of `make go-test` at `3b9d82bd`: `/tmp/claude-0/-home-user-plux3/48b3b72a-2abb-5762-a1af-a581b93bd4ab/scratchpad/gotest-3b9d82bd.log` — grep it, never dump it):
- `internal/benchproject` `TestRuntimeBaseline`; `internal/bundle` `TestVerifierVectors` — likely conformance goldens out of date after the bundle/experiment changes or the 0.6.0 runtime bump: regenerate with the repo's `-update` flags only where the output legitimately changed, and say why.
- `internal/httpx` `TestNoUnregisteredOutboundCall_SEC_163` — a new outbound client lacks an egress `Destination` (E5j's file exports or E9e's push provider): register it like its neighbours, regenerate `docs/operations/egress.md` with `make gen`.
- `internal/release`: `TestReconstruction*_SEC_142`, `TestAuditChannelIsNeverServed_SEC_142`, `TestRolloutEndRevertsOnce_REL_052` — find the cross-slice cause (likely the approval gate, restriction policies or the runtime bump) and fix the code or the test fixture, whichever is wrong; never weaken an assertion.
Root-cause each; one commit per cause. PostgreSQL is running (`PLUX_TEST_DATABASE_URL` set).
Done when: `cd backend && go test -count=1 ./internal/benchproject/ ./internal/bundle/... ./internal/httpx/... ./internal/release/...` passes, then `make go-test` passes once.
