<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# P9 hand-off 2 (2026-10-21, 09:30 UTC)

The session that ran P9 from 2026-10-19 to 2026-10-21 stopped here on the maintainer's request, to continue in a new session **exactly as it was working**: the lead (one session) writes briefs, launches 5–6 sub-agents at a time, reviews and fixes every result, merges into `feat/phase-9`, and runs the gates once at the end. This file is everything the next lead needs; nothing lives only in the old session.

**Read first, in this order:** `AGENTS.md`, `docs/WORKLOG.md` (latest hand-off note), this file, `agents/AGENT-PLAYBOOK.md` (the maintainer's binding playbook), `agents/prefix.md` (the rules every agent gets), then the plan `docs/plans/p9.md` sections a task needs. The first hand-off (2026-10-18, P8 close and P9 start) is kept as `README-2026-10-18.md`; its patches were consumed and deleted.

---

## 1. State in one paragraph

`feat/phase-9` (pushed) holds P8 plus everything of P9 below. P9 is **≈ 72 %**: E0, E1, E10 complete; E3, E4, E6, E7, E8 mostly complete; E5 and E9 well advanced; E2 (approvals) under way; E11 (reference apps, walkthrough, docs) and E12 (gates) not started. Two agents were mid-task when the session stopped; their work is saved on pushed branches. No agent is running. **The full gate (`make check`) has not been run in P9**; every merge was verified by building both Go modules, `go vet` and the touched packages' tests (see §8).

## 2. Branches

| Branch | State |
|---|---|
| `feat/phase-9` | **The lead branch.** Everything merged; pushed. Continue here. |
| `wip/p9-e2a-approvals` | E2a approval engine, in progress (one save commit on an older base). Pushed. |
| `wip/p9-e9e-load` | E9e load generator / sizing / bench, in progress (two commits). Pushed. |
| `wip/p9-handoff` (remote) | Obsolete (the first hand-off). Delete once the maintainer agrees. |
| `claude/tender-mendel-b4bh9u` (remote) | Obsolete (an early copy of `feat/phase-9`). Delete once the maintainer agrees. |
| `main` | P8 (`e0a5fe6`) plus three later commits (#37 scorecard bump, #39 CI function modules, #40 Go dependency group bump) **not yet merged into `feat/phase-9`** — merge `origin/main` into `feat/phase-9` once (a merge commit; regenerate with `make gen`; run the Go tests). |

## 3. Done and merged (this session)

Migrations are contiguous `0024`–`0043`; the next free number is **`0044`**. The brief of every slice below is kept in `agents/briefs/done/` as it was sent (a few name the old session's scratch directory; ignore those paths).

| Slice | Requirements | What landed | Lead fixes at review/merge |
|---|---|---|---|
| E9b backups, drill, observability | DEP-010, OBS-004, OBS-005 | CNPG WAL archiving + ScheduledBackup + consistency marker; `make restore-drill`; 38 alerts with runbooks under `docs/runbooks/alerts/`; 5 infra dashboards; SLO burn-rate rules; checker `tools/internal/observability` | `plux_publish_jobs_total` was never recorded — release service now counts finished jobs |
| E7 notifications, webhooks, triggers | SRV-062, SRV-063, FN-025 | `internal/notify`, Standard Webhooks, cron + inbound hooks, `internal/scheduler` | — |
| E5 contracts + compiler | ABT-001/002/005/006 (compile time) | `Variant` node, flag/segment/experiment/policy schemas, diagnostics 1700–1704/2300–2302, `PromoteWinner` | winner promotion rewritten on the PXL parser (`pxl.ReplaceMembers`) instead of a hand-written scanner; codegen test counts 17 documents |
| E8a SIEM export | SEC-143 | `internal/auditexport`, syslog RFC 5424/TLS core; CEF, OTLP logs, signed webhooks in `ee/backend/auditexport` | — |
| E8b search, Git export | SRV-061, GOV-011 | `internal/search` (filtered in SQL), `internal/gitsync` (go-git), CLI | — |
| E6a telemetry + analytics | ANL-010, ANL-011 | Valkey streams, monthly partitions, rollups, retention with off-peak window, OTLP export, CLI | partition drop moved from a migration into Go (`pgx.Identifier`) so migrations stay expand-only |
| E3 control document (server) | SYN-060, REL-060, REL-070 | `schema/control.fbs`, online `control` role, `GetSignedControl` (ETag, DPoP), `plux control`, FCM/APNs senders | — |
| E1b federation | GOV-004 | OIDC providers/mapping/JIT/back-channel (core); SAML (`ee/backend/saml`), SCIM (`ee/backend/scim`) | person-scoped sign-in tables listed as non-tenant |
| E6b dashboards | ANL-020, I18N-006 | 8 analytics dashboards over `analytics_rollups`, reader datasource | **SQL injection**: template variables were pasted raw into SQL — now `${var:sqlstring}`, checker refuses hand-quoted variables |
| E3 runtime | SYN-060, REL-060, REL-070, RT-022 | fetch/verify/apply on device, `PluxConfig.updateScreen`, `Plux.handleSilentPush` | dedicated codes PLX-3105–3107 for switched-off graph/source/function (they reused the plugin code); golden-bundle test learns the experiments bundles |
| E8c egress registry | SEC-163 | every outbound client names a registered `Destination`; generated `docs/operations/egress.md`; completeness scan | — |
| E8d data-subject requests | SEC-162 | export/erase, tombstones, generated `docs/operations/personal-data.md` + completeness test | map extended for later tables (step-up, error groups, reconstruction) |
| E1c sessions + step-up | GOV-005, SEC-103 | idle/absolute timeouts, revocation, WebAuthn step-up, CLI browser step-up grant bound to the DPoP key | admin fixtures in notify/search/gitsync/analytics step up |
| E6b alerts + error grouping | ANL-030, ANL-040 | alert rules (River unique period), error groups with lazy symbolication | — |
| E1d federation tests | GOV-004 | core tests for providers, mapping, JIT, back-channel logout, Directory | the agent found and fixed a real bug: every SCIM request failed (token touch under the wrong tenant scope) |
| E5 runtime | ABT-002/004/006, NAV-009 | assignment, layers, holdout, Variant rendering, exposure on build, flag guards | — |
| E4 rollouts | REL-050–053 | bucketing, targeting at manifest time, schedules, health gates, CLI | (agent's commit was refused by its permission check; the lead committed) |
| E8e reconstruction (server) | SEC-142 | resolve the moment, signed release on the `audit` channel, `plux audit reconstruct` | dev-release retention keeps releases a reconstruction references; test world's request allowance raised (the matrix now exceeds 600 calls/min) |
| E5 server | ABT-001/003/004/005/006 | flags, experiments, layers, holdout, in-house mSPRT/SRM/CUPED, promotion, `ApprovalGate` seam | — |
| E6c ClickHouse, Parquet (ee) | ANL-010, ANL-011 (ee) | `ee/backend/analytics`: ClickHouse store (verified live against ClickHouse 25.8), Parquet and webhook exporters, store conformance suite `backend/ext/analytics/storetest` | — |
| E9c air-gapped bundle (ee) | DEP-040 | `ee/backend/cmd/plux-airgap` (keygen, bundle, verify), OCI layout, signed manifest, `make airgap-kind`, `docs/operations/air-gapped.md` | — |
| E9d IaC | DEP-050 | OpenTofu modules `deploy/iac/{aws,gcp,azure}` with `mock_provider` tests, `policy.sh`, `make install-tofu` (built from the Go module), `make iac-check` | — |

Earlier (first hand-off): E0, E9a Helm chart, E10 `ee/` + licence, E1a roles and catalogue.

## 4. In progress (agents stopped by the usage limit; nothing running)

| Slice | Branch | Brief | State when stopped |
|---|---|---|---|
| E2a approval engine core | `wip/p9-e2a-approvals` | `agents/briefs/in-progress/E2a-approvals.md` | ~61 files written; was "writing the CLI test". Base is older than `feat/phase-9` (expect wiring conflicts). |
| E9e load, sizing, bench-control | `wip/p9-e9e-load` | `agents/briefs/in-progress/E9e-load-sizing.md` | generator and propagation metric committed; was "applying the server edits". |

Relaunch them with the continuation briefs in `agents/briefs/next/` (§5); they read the original brief themselves.

## 5. Next agents, in order (none started)

Launch 5–6 at a time; the model is chosen for the job (Haiku where the lead can verify everything, Sonnet otherwise; never Opus for agent code). Each prompt is two lines (template in §6).

| # | Brief (`agents/briefs/next/`) | Agent type | Depends on | Notes |
|---|---|---|---|---|
| 1 | `E2a-approvals-continue.md` | coder-high | — | security; also implements E5's `ApprovalGate` (ABT-007) |
| 2 | `E9e-load-continue.md` | coder | — | |
| 3 | `E5i-runtime-integration.md` | coder | — | Dart: control tables → models, device targeting, reconstruction inputs, translations |
| 4 | `E5j-server-integration.md` | coder | — | Go: prop defaults in bundle, typed flag accessors, error-rate series, ee export scheduling, generated control layout, purge test |
| 5 | `E8f-viewer-host.md` | coder | — | `apps/plux_viewer` + runtime reconstruction mode |
| 6 | `E2b-breakglass-freezes-compliance.md` | coder-high | E2a merged | break-glass core; freezes + compliance reports ee; in-house PDF |
| 7 | `E11b-reference-apps.md` | coder | E2a, E2b, E5i/j | |
| 8 | `E11c-walkthrough.md` | coder-high | everything above | the exit criterion |
| 9 | `E11a-docs.md` | coder-light | everything above | runbooks, guides, threat model, compliance pack |

Then the lead's own work (§7).

## 6. How to run it (the practice that worked, and its traps)

**Session start.**
1. Install the agent types (they load only at session start — if you install them mid-session, use `subagent_type: general-purpose` with `model`/`effort` instead):
   `mkdir -p ~/.claude/agents && for f in coder coder-high coder-light; do tail -n +6 docs/plans/p9-handoff/agents/definitions/$f.md > ~/.claude/agents/$f.md; done`
2. Put your own session URL into `agents/prefix.md` (the `Claude-Session:` line) and use the commit attribution your session's system reminder gives.
3. `git fetch && git checkout feat/phase-9 && git pull`; merge `origin/main` (§2).

**Launching.** `Agent(subagent_type=<type>, isolation=worktree, run_in_background=true)` with a prompt of exactly:
> First read, completely, the binding rules `docs/plans/p9-handoff/agents/prefix.md` and your task `docs/plans/p9-handoff/agents/briefs/next/<file>.md`. Then do the task.

This keeps the lead's output tokens tiny and the agents' shared prefix identical. Agents report in ≤ 8 lines; do not converse with them beyond "continue" after a limit or a one-line correction.

**Traps seen and how to handle them.**
- **Worktree base:** agent worktrees are created from the default branch (`main`), not `feat/phase-9`. The prefix tells agents to `git reset --hard feat/phase-9` when the fast-forward fails; check the first report line.
- **Usage limits** stop all agents at once (resets every ~5 h). Their work is saved in their worktrees; resume each with `SendMessage(to=<agentId>, "The usage limit has reset: continue from where you stopped.")`.
- **Container restarts** kill agents silently (no notification) and mark them "stopped by the user" — they cannot be resumed. Check liveness with `uptime` and file mtimes in `.claude/worktrees/agent-*`; save their work (`git add -A && git commit` in the worktree, never `--no-verify`), rename the branch `wip/p9-<slice>`, and relaunch a continuation brief.
- **Agents' own commits** are sometimes refused by their permission check; commit for them in their worktree.
- **Merging:** do merges in a separate lead worktree (`git worktree add -B lead/merge <scratch>/lead feat/phase-9`), then `git merge --ff-only lead/merge` in the main checkout. **Never edit the main checkout while a push runs**: the pre-push hook runs `make go-test` on the working tree and a half-resolved merge fails it (this cost two pushes).
- **Conflicts** are almost always the shared wiring lists: `backend/cmd/plux/{main.go,completion.go}`, `backend/internal/api/{handler.go,authz_matrix_table_test.go,authz_matrix_integration_test.go,services_integration_test.go}`, `backend/internal/server/{services.go,build.go}`, `backend/serverapp/main.go`, `ee/backend/cmd/plux-server-ee/main.go`, `backend/internal/audit/audit.go`, `docs/engineering/dependencies.md`. Keep both sides. In `permissionsWithoutRPC` (authz_matrix_integration_test.go) a conflict usually means *both* sides removed entries — the result is neither. For generated files (`dbgen/`, `*.pb.go`, `openapi.yaml`, `docs/reference/*`) take either side, then `make gen` with `PATH=$PATH:/root/.cache/plux/flatc-25.9.23`. A Python resolver that opens the file for writing before the substitution can empty it on an exception — `git checkout -m <file>` restores the conflict.
- **Migrations:** every agent takes the next free number on its base, so collisions are routine; renumber at merge with `git mv` in merge order (nothing deployed reads the numbers yet; the runner requires contiguity).
- **Cross-slice gaps** found only at merge: new tables must be added to the personal-data map (`backend/internal/dsr/datamap.go` + export/erase executors), new person-scoped tables to `nonTenantTables`, new admin fixtures need `.WithStepUp(0)`, new RPCs need matrix entries. The completeness tests catch them — run `./internal/dsr/ ./internal/storage/ ./internal/api/` after every merge.
- **Disk** fills (Go cache, parallel test temp dirs): `go clean -cache; rm -rf /tmp/go-build*`. PostgreSQL then fails with "could not write init file" — that is the disk, not the code.
- **Killing a background push:** kill by PID (`pgrep -f "git push origin feat/phase-9"`); `pkill -f` with a pattern also matches your own shell.

**Review standard.** The lead reads every security-relevant part of a diff (signing, auth, SQL built from input, SSRF, secrets, migrations) and spot-checks the rest; it raises to the maintainer only wrong behaviour or large debt, with a recommendation, and otherwise fixes and merges.

## 7. Lead-only work still to do

1. Merge `origin/main` into `feat/phase-9` (§2).
2. CI (`.github/workflows/ci.yml`, `ci/affected.json`), after the maintainer's decision on §9.3/§9.4: nightly + path-selected jobs for `make restore-drill`, `make airgap-kind`, `make iac-check`, `make walkthrough-kind`, `make load-test`, `make bench-control` (when the IaC job is added, move `mk/iac.mk` out of `noJob` in `ci/affected.json` into it; it is in `noJob` only because no job runs it yet); the `ee` build/test job; keep `make go-test-core`. `make workflows-lint` (use `ZIZMOR_FLAGS=--offline` here).
3. Run the Dart suites that never ran to completion after the E5-runtime merge (they timed out under load): `cd packages/plux_flutter && flutter test test/experiments test/control test/bundle test/pxl test/render`.
4. E12: spec statuses in `docs/requirements.md` (only the `Status` column; `make spec-lint`), `make trace` (0 violations), `make check` with every gate, `make e2e-starter`, 40× race runs of `TestPublishWithTheCLIAndSyncADevice`, a dispatched CI run green, `docs/WORKLOG.md`, the PR body. Coverage floors for the new packages (plan §8, A6: 85 % security-relevant — auth, approval, control, licence, egress — 80 % elsewhere) go into `coverage.json`.

## 8. What has and has not been verified

**Verified at each merge (this session):** `go build ./...` in `backend` (with and without `GOWORK=off`) and `ee/backend`; `go vet` of touched packages; `go test -count=1` of the merged and neighbouring packages against PostgreSQL (api, server, storage, dsr, release, auth, cmd/plux and the slice's own); `dart analyze lib test` (clean); the pre-push `make go-test` on every push.

**Not verified:** `make check` (lint, coverage floors, govulncheck, reproducibility, REUSE over everything, spec lint after statuses); `-race` across the board; the full Flutter suite; `make e2e-starter`; any `kind`, drill, air-gapped or walkthrough run (no Docker here); OpenTofu `init/validate/test` (provider registry blocked here — CI only); CI on `feat/phase-9`.

## 9. Open decisions for the maintainer (with the lead's recommendation)

1. **SCIM deprovisioning and sessions** — deprovisioning revokes the person's tokens in the organisation at once and ends their sessions only when they belong to no other organisation (sessions are user-wide). ADR-0074 says "revokes sessions at once". *Recommend:* accept and note it in ADR-0074.
2. **Chart egress default** — `networkPolicy.externalHTTPS.enabled` defaults to `true` (api and worker may reach any HTTPS host), which arguably conflicts with SEC-163 "by default no data leaves". *Recommend:* default `false`, enabled explicitly with the egress list; it is a security default, so it needs approval.
3. **CI jobs** for IaC (`make iac-check`; the provider registry is blocked in agent sessions) and the air-gapped kind install. *Recommend:* add (nightly + path-selected).
4. **New tools** for `make airgap-kind`: `skopeo` and the `registry:2` image need `dependencies.md` rows. *Recommend:* approve.
5. **Smaller items accepted by the lead, for the maintainer's awareness:** SAML refuses SHA-1, PKCS#1 v1.5 key transport and documents with comments; ClickHouse retention runs through `Purge`/`Compact` instead of table TTL (retention is per organisation); experiment analysis reads raw events (rollups carry no per-subject variance); flags and experiments are scoped per environment; health gates default to a 200-sample minimum and a 6-hour window (per-rollout overrides; not registry limits); `retention.manage` is owner-only; the `admin` role lacks `killswitch.manage` (the release manager has it); Grafana analytics dashboards are an installation-operator tool (any Grafana viewer may pick any organisation); same-version control documents are accepted idempotently on the device; two parser bounds are constants, not registry limits (`ee/backend/saml/xml.go` depth 64, `ee/backend/airgap/layout.go` 16 MiB).
6. **Limit keys added in P9 since the first hand-off** (defaults to confirm, with the ten of the first hand-off): `audit.exportBatchEntries`, `audit.exportDestinationsPerOrg`, `auth.samlClockSkewSeconds`, `auth.scimTokenOverlapSeconds`, `auth.sessionAbsoluteTimeout`, `auth.sessionIdleTimeout`, `auth.stepUpAttempts`, `control.expiry`, `control.killSwitches`, `push.batchSize`, `push.tokenBytes`.
7. From the first hand-off, still open: the ten limit defaults; the ADR-0066 native-slot locale rule; the legal review of the draft `ee/LICENSE`.

## 10. Known follow-ups not yet in a brief

- ClickHouse writes are gated at `Open` only (a licence expiring mid-run does not stop writes).
- The production bundle-signing public key for `plux-airgap verify` is not compiled in (`--key` needed).
- `DeleteUnreferencedVersions` is safe now only because releases referenced by reconstructions are kept; reconstructions expire on `reconstruction.retentionDays`.
- Rollout user attributes accept hashes only (no allowed plaintext attributes).
- Promotion of an experiment winner is not atomic across services (the error says so).
- The chart has no KMS signing backend values; Azure Blob has no S3 API for the chart's object storage (IaC README notes both).

## 11. Progress (lead's estimate)

| Milestone | % | Left |
|---|---|---|
| E0, E1, E10 | 100 | — |
| E7 | 95 | gates |
| E3 | 90 | `bench-control` (E9e) |
| E6 | 90 | export scheduling (E5j) |
| E4 | 85 | device targeting (E5i), error-rate series (E5j) |
| E8 | 85 | viewer host + reconstruction mode (E8f) |
| E5 | 75 | integration (E5i, E5j), ABT-007 (E2a) |
| E9 | 75 | load/sizing (E9e), CI runs |
| E2 | 35 | E2a finish, E2b |
| E11 | 0 | E11a–c |
| E12 | 0 | §7 |
| **P9** | **≈ 72** | |

Numbers on `feat/phase-9` since `main`: 383 commits, 963 files, +184 k / −16 k lines, migrations 0024–0043, 20 agent slices merged this session.
