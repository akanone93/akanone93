<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# P8 close-out and P9 hand-off (2026-10-18, 17:40 UTC)

The session that ran P8 and started P9 stopped here on the maintainer's request, to move the
work to a new session. This directory holds everything that was in progress. It lives on the
temporary branch `wip/p9-handoff` (cut from `feat/phase-9` at `938e7b1`); delete the branch
and this directory once the work is merged.

Read first, in this order: `AGENTS.md`, `docs/WORKLOG.md`, `docs/plans/p9.md` (decisions §2,
allocations §2.4, architecture §5, milestones §6), then this file.

---

## 1. Branches

| Branch | Head | State |
|---|---|---|
| `feat/phase-8` | `796ddbd` | P8 complete. Waiting on CI run 39149009669 (§2). |
| `feat/phase-9` | `938e7b1` | Cut from `feat/phase-8`, P8 merged in; E0 merged (§3). |
| `wip/p9-handoff` | this commit | `feat/phase-9` + this directory: the in-progress P9 slices as patches. |

`feat/phase-9` was cut from `feat/phase-8`, not from `main`, because P8 was not merged yet.
After P8 is squash-merged, merge `main` into `feat/phase-9` (conflicts are P8's own content
on both sides; take `main`'s side).

---

## 2. P8: what remains

1. **CI run 39149009669** on `796ddbd` (dispatched 17:19 UTC). Run 322 on `c0479a3` was green
   in 47 of 49 jobs; only "Device end-to-end (iOS, bank)" failed. Its diagnostics showed the
   cause: XCTest's first key into a Flutter text field arrives twice (the username read
   `ddemo`), so every iOS bank sign-in had a wrong username. `796ddbd` types one key at a
   time, reads the field after each key and deletes what a key added beyond itself
   (`apps/plux_bank/ios/RunnerUITests/AccessibilityTreeTests.swift`, `type(_:_:)`); it also
   checks that the PIN pad reads "6 of 6 digits entered". Every Android shard (26 and 35),
   iOS express+generated, iOS starter and iOS hosts are green on the earlier runs.
2. **If the run is green:** open the pull request `feat/phase-8` → `main`, title
   `feat: localisation and accessibility (P8)`, body [`p8-pr-body.md`](p8-pr-body.md)
   (update the run number in "How it was verified" to the green run). The maintainer merges.
3. **If iOS bank fails again:** read the XCTest assertion (`grep ": error: -\["` in the job
   log; the driver prints it, and a timed-out wait lists what the screen reads), fix in
   `AccessibilityTreeTests.swift` on `feat/phase-8`, push, re-dispatch
   (`actions_run_trigger run_workflow ci.yml ref feat/phase-8`), then merge `feat/phase-8`
   into `feat/phase-9`.
4. **Gates:** no CI gate was lowered in P8 (`coverage.json` unchanged; `ci.yml`, the
   Makefile and `mk/` only gained checks). Narrowed test scopes, all documented in their
   code: the Android bank pass is skipped below API 28 (Flutter names text fields to
   Android only from API 28), the transfer page's reading order is checked on its first
   three entries, and `go-determinism` skips `plux golden`'s CLI tests (run in the Go jobs).
5. **Decisions answered by the maintainer at the end of P8:** `session_start`,
   `session_end`, `fatal_error` and `control_applied` are `necessary` (no consent needed);
   `experimentApproval` follows `ABT-007` under `maximum` (committed).
6. **Open for the maintainer:** confirm the defaults of the ten new limit keys in
   `schema/limits.json` (`telemetry.eventsPerDevicePerHour` 600, `telemetry.batchBytes`
   256 KiB, `experiments.activePerApp` 20, `flags.perApp` 500, `rollouts.activePerApp` 10,
   `webhooks.perOrg` 50, `notifications.perHour` 600, `reconstruction.retentionDays` 365,
   `search.resultsPerPage` 50, `dsr.exportBytes` 256 MiB); keep or drop the compiler rule
   that keeps all locale data when an app declares native slots (ADR-0066 Revision).

---

## 3. P9: merged on `feat/phase-9` (milestone E0, complete)

| Item | Commit(s) |
|---|---|
| P9 plan with the maintainer's decisions Q1–Q24 and approvals A1–A6 | `093f2df`, `c5f5809` |
| Spec 1.7.0: phase P16, `HST-020` → P16, `SEC-191` → P12, `DX-005`/`DEP-060` withdrawn, §15.12 settings, §18.2 default policies, Appendix F ranges, Appendix G.2 events; spec lint accepts P16 | `86c79e2` (+ `0ed370f`) |
| ADR-0072 to ADR-0085 (Accepted) and revisions of ADR-0004, 0007, 0022, 0026, 0034 | `afb9062` |
| 56 error codes and 10 limit keys (generated catalogues regenerated) | `a13b84f` |
| `DualCalendarDatePicker` opens on the current month (Q24) | `2cd1a49` |
| Locale data trimmed to what the app uses (ADR-0066 Revision; bank −19%, express −41% of l10n bytes; goldens and vectors regenerated) | `aad6ec6`, `938e7b1` |

Verified on `938e7b1`: `go build ./...`; `go test` of `plxerr`, `api`, `compiler`, `l10n`,
`pxl`, `bundle`, `cmd/plux`; `make go-determinism`; `make spec-lint`; `make go-gen-check`;
`make reuse-lint`; the pre-push `go test`. Not run: `make check` in full, the Dart suites
beyond the touched tests, CI on `feat/phase-9`.

---

## 4. P9: in progress (stopped mid-slice), saved as patches

Each patch is the slice's whole work (its commits plus uncommitted files) against the
commit it started from. None of them was reviewed, built or tested by the lead. To resume a
slice: create a worktree at its base, `gunzip -c <patch>.gz | git apply --3way`, read the slice's brief
(§5), finish the remaining steps, run its checks, then merge into `feat/phase-9`.

| Patch | Milestone | Base | Size | Where it stopped |
|---|---|---|---|---|
| [`E9a-helm.patch.gz`](E9a-helm.patch.gz) | E9: Helm chart, HA topology, `kind` target, tool pins (`DEP-003`, `DEP-004`) | `0ed370f` | 28 files, +3834 | Chart, values schema and templates largely written; was dropping `ct` (chart-testing) from `lint.sh` because it could not be installed; CI jobs and docs to check |
| [`E10-ee-licence.patch.gz`](E10-ee-licence.patch.gz) | E10: `ee/` module, `backend/internal/edition`, licence verification, `plux-license`, `ee/LICENSE` draft (`GOV-030`, `GOV-033`) | `fcafe9a` | 61 files, +3855 −282 | Was writing the ee server binary (`ee/cmd/plux-server-ee`); CLI, CI core-without-ee job and tests to finish |
| [`E1a-roles.patch.gz`](E1a-roles.patch.gz) | E1: permission catalogue, ten roles, teams, scoping, custom roles, `rbac_p9.go`, migrations 0024–0025, `governance.proto` (`GOV-002`, `GOV-003`) | `fcafe9a` | 38 files, +7098 −266 | Was adding the edge test at the end of the authorisation-matrix integration file |
| [`E6a-telemetry.patch.gz`](E6a-telemetry.patch.gz) | E6: ingestion through Valkey streams, partitioned storage, rollups, retention, OTLP export, migrations 0039–0041 (`ANL-010`, `ANL-011`) | `fcafe9a` | 55 files, +7569 −200 | Was editing `telemetry.proto` and writing `analytics.proto` |
| [`E7-notify.patch.gz`](E7-notify.patch.gz) | E7: notifications, Standard Webhooks, cron and inbound-webhook function triggers, migration 0042 (`SRV-062`, `SRV-063`, `FN-025`) | `fcafe9a` | 26 files, +6838 | Was writing `triggers.go` (the notify import added) |
| [`E1c-stepup.patch.gz`](E1c-stepup.patch.gz) | E1: sessions, idle/absolute timeouts, WebAuthn step-up, CLI browser step-up, migrations 0026–0027 (`GOV-005`, `SEC-103`) | `938e7b1` | 7 files, +187 | Early: first files only |
| [`E3-control.patch.gz`](E3-control.patch.gz) | E3: signed control document, kill-switch scopes, mandatory update, forced rollback, silent push, migrations 0032–0033 (`SYN-060`, `REL-060`, `REL-070`) | `938e7b1` | 3 files, +26 | Just started; effectively not begun |

**Shared-file conflicts to expect when merging these:** `backend/internal/auth/rbac_p9.go`
(E1a creates it with all P9 permissions; E6a, E7, E1c and E3 were told to append their own
const blocks if it did not exist — reconcile into E1a's version), `go.work` (E10 adds
`ee/backend`), `Makefile`/`mk/*.mk` and `.github/workflows/ci.yml` (E9a, E10),
`docs/engineering/dependencies.md` (E9a), the migration numbers (each slice has its own block
of plan §2.4), generated code (re-run `make gen` after each merge; never hand-merge generated
files).

---

## 5. The briefs (what each slice must deliver)

Every brief began with rule 0 (`git merge --ff-only feat/phase-9`), committed in its
worktree with the two attribution lines, never pushed, and never used `--no-verify`. The
detailed requirements are in the plan section named; the summaries below list what "done"
means.

- **E9a (coder-high)** — plan §5.11.1–2, §7, A3–A4. Pin Helm, kind, kubeconform, ct
  (Go-installable where downloads are blocked) in the Makefile like the other pinned tools;
  `dependencies.md` rows for them and for CloudNativePG, Valkey (Sentinel), SeaweedFS.
  `deploy/helm/plux/`: Chart.yaml, values.yaml, strict `values.schema.json`, templates
  mirroring `deploy/compose` (api, worker, fnrunner, egress, builder), existingSecret pattern,
  default-deny NetworkPolicies (fnrunner/builder reach only egress; L-4), Pod Security
  `restricted`, PDBs, HPAs, topology spread, probes, checksum annotations, pre-upgrade
  migration Job, `helm test` hook; HA defaults (≥ 2 replicas, CNPG 3 instances,
  `postgresql.mode: cnpg|external`, `valkey.mode: sentinel|external`, object storage
  `seaweedfs|external`). Make targets `helm-lint` (lint + template + kubeconform over default,
  HA and external value files in `deploy/helm/plux/ci/`) and `e2e-kind`. CI: "Helm lint and
  render" job and a nightly/path-selected "kind HA e2e" job with `ci/affected.json` rules;
  `make workflows-lint`. Docs: chart README, note in `deploy/README.md`.
- **E10 (coder)** — plan §2.2 D-L, §2.6, §5.10. `ee/backend` module in `go.work`; core
  `backend/internal/edition` (feature constants for the right column of §2.6,
  `Enabled`/`EnabledForWrite`/`Readable`, PLX-8600; no licence → all ee features off; core
  builds without ee). `ee/backend/license`: verify the signed licence in the update-metadata
  envelope (Ed25519, compiled-in keys by ID, test key pair in testdata, production key slot
  left empty), states valid/grace/expired/invalid/absent, hourly re-verification, metric
  `plux_license_days_remaining`, limits only for registry keys, never gating security
  (`GOV-032`). `ee/cmd/plux-license` (keygen, sign, verify). `ee/cmd/plux-server-ee`;
  `PLUX_LICENSE_FILE` config. CLI `plux license inspect`, `plux admin license`
  (`license.proto`). `ee/LICENSE` draft "Plux Enterprise License 1.0" marked for legal
  review; `REUSE.toml` `ee/**` → `LicenseRef-Plux-Enterprise` with
  `LICENSES/LicenseRef-Plux-Enterprise.txt`. A `make go-test-core` (core without ee) wired
  into CI. Tests for every licence state; coverage ≥ 85%.
- **E1a (coder-high)** — plan §5.1, §2.4. Generated permission catalogue (description,
  risk, `stepUp`) and `docs/reference/permissions.md`; `auth/rbac_p9.go` with all P9
  permissions; the ten roles (existing grants unchanged, proven by a test); teams; grants
  `(principal, role, scope)` with scope organisation | app | app+environment and
  `<permission>:<environment>`; custom roles (gated by `edition.Enabled(FeatureCustomRoles)`
  once E10 is merged); codes 8100–8103; migrations 0024–0025; `governance.proto`; CLI
  `plux roles|teams|grants`; regenerated authorisation matrix (`QA-008`).
- **E6a (coder)** — plan §5.8, Q17, §2.4. Valkey-stream ingestion (extend the in-house
  RESP client), bulk writes by the worker, quotas (PLX-8300), server-side consent check,
  direct-write fallback without Valkey; monthly partitions; 5-minute/hourly/daily rollups
  with histogram percentiles; `analytics.Store` interface (PostgreSQL now, ClickHouse later in
  ee); retention defaults (13 months raw, rollups indefinitely, 10 years production audit)
  configurable per organisation and data class, applied in the off-peak window (default
  02:00–04:00) or immediately, schedulable and cancellable, audited (PLX-8301); OTLP export
  through the SSRF-safe client; `analytics.proto`; CLI `plux analytics`; migrations
  0039–0041.
- **E7 (coder)** — plan §5.7, §2.4. `backend/internal/notify` with the event registry,
  subscriptions per user and team, inbox, SMTP (STARTTLS), Slack, Teams, Telegram, per-hour
  limit, delivery log; Standard Webhooks (headers, `whsec_` secrets with rotation overlap,
  24 h retry with backoff and jitter from a PostgreSQL queue, PLX-8401, the published test
  vectors); cron triggers (5-field + macros, zones, DST-correct, advisory-lock leader) and
  inbound webhooks `/hooks/{app}/{function}` (Standard Webhooks or a `Verifier`
  interface, replay protection, PLX-7150–7152) invoking through the function service's
  workload identity; migration 0042; `notification.proto`; CLI.
- **E1c (coder-high)** — plan §5.2, Q15, §2.4. Sessions table, idle 30 min / absolute 12 h
  defaults per organisation, listing and revocation (immediate), PLX-6230–6232; WebAuthn
  step-up with user verification recorded as `stepUpAt`, `auth.RequireStepUp(ctx, 5*time.Minute)`
  on the stepUp actions (production promotion, keys, members/roles, secrets, security
  configuration, function production deploy — replacing P7's stand-in without loosening it),
  PLX-6200–6202; CLI browser step-up (request bound to the CLI's DPoP thumbprint, page with
  `navigator.credentials.get`, 5-minute grant, polling with DPoP, audited); migrations
  0026–0027; `identity.proto` additions; CLI `plux sessions`.
- **E3 (coder-high)** — plan §5.4, Q9, Q22, §2.4. `schema/control.fbs` (version, expiry,
  kill-switch scopes plugin|page|action graph|data source|function with targeting
  placeholders, mandatory update, forced rollback, reserved slots for flags and experiments)
  in the update-metadata envelope signed by the new online `control` role through
  `signing/`; migrate the unsigned per-channel `Control` (migration 0032); ETag + DPoP
  endpoint; FCM HTTP v1 and APNs senders in-house, push tokens (migration 0033), off by
  default, destinations listed for the egress list. Runtime: fetch on resume and every 5
  min off the UI isolate, verify before parse, refuse older versions (PLX-3102), apply
  without restart with fallbacks (`RT-022`), keep switches in force past expiry (PLX-3101),
  mandatory update behind `PluxConfig.updateScreen` (PLX-3103), forced rollback
  (PLX-3104) with `control_applied`, `Plux.handleSilentPush`.

Not started: E2 (approvals), E4 (rollouts), E5 (flags and experiments), E6b (ClickHouse,
Parquet, dashboards, alerts, error grouping), E8 (audit export, reconstruction, DSR, egress,
search, Git export), E9b (backups, air-gap, observability, sizing, IaC), E1b (OIDC
extension, SAML, SCIM), E11, E12 — plan §6.

---

## 6. How the work was run (for the next session)

- The lead merged agent branches with a helper that took `ours` for generated files and
  re-ran `make gen` afterwards; generated files are never merged by hand.
- Agent worktrees start from `main`; rule 0 brings them to `feat/phase-9`.
- CI runs only on dispatch (`ci.yml`, `workflow_dispatch`) for these branches; a newer
  dispatched run in the same concurrency group can cancel an older one.
- The pre-push hook runs `make go-test`; do not run `make gen` or clean caches while a push
  runs.
