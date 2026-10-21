<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Task list snapshot (2026-10-21, hand-off 2)

The lead's task list at hand-off 2. Details for every row are in [README.md](README.md);
the earlier snapshot (2026-10-18) is in git history. Status words: `done` (merged on
`feat/phase-9`), `in progress` (stopped; work saved on a `wip/` branch), `next` (brief
ready, not started), `lead` (the lead's own work, no agent).

| # | Task | Status | Where |
|---|---|---|---|
| 1 | E0: spec 1.7.0, ADRs 0072–0085, error codes | done | README §3 |
| 2 | E1: roles, scoping, OIDC, SAML, SCIM, sessions, step-up | done | README §3 |
| 3 | E2a: approval engine (+ E5 `ApprovalGate`, ABT-007) | done (session 3; + ABT-007 gate) | `wip/p9-e2a-approvals`; brief `agents/briefs/next/E2a-approvals-continue.md` |
| 4 | E2b: break-glass, freezes, compliance reports | running (session 3) | `agents/briefs/next/E2b-breakglass-freezes-compliance.md` |
| 5 | E3: signed control document, kill switch, mandatory update, silent push | done | README §3 (bench in row 7) |
| 6 | E4/E5: rollouts, health gates, flags, experiments — core | done | README §3 |
| 7 | E9e: load tests and sizing (incl. `bench-control`) | done (session 3; propagation query cached by the lead) | `wip/p9-e9e-load`; brief `agents/briefs/next/E9e-load-continue.md` |
| 8 | E5i: runtime integration (Dart) | done (session 3) | `agents/briefs/next/E5i-runtime-integration.md` |
| 9 | E5j: server integration (Go) | running (continuation, session 3) | `agents/briefs/next/E5j-server-integration.md` |
| 10 | E6/E7: telemetry, analytics, notifications, webhooks, triggers | done | README §3 (export scheduling in row 9) |
| 11 | E8: SIEM, reconstruction, DSR, egress, search, Git export — core | done | README §3 |
| 12 | E8f: viewer host + runtime reconstruction mode | done (session 3; + E8g trust anchor, E5k switch targeting) | `agents/briefs/next/E8f-viewer-host.md` |
| 13 | E9: Helm/HA/backups/air-gap/observability/IaC | done except row 7 | README §3 |
| 14 | E10: `ee/` + licence | done | README §3 |
| 15 | E11b: reference apps | next (after 3, 4, 8, 9) | `agents/briefs/next/E11b-reference-apps.md` |
| 16 | E11c: end-to-end walkthrough (exit criterion) | next (after all above) | `agents/briefs/next/E11c-walkthrough.md` |
| 17 | E11a: docs, runbooks, threat model, compliance pack | next (last agent) | `agents/briefs/next/E11a-docs.md` |
| 17a | E1e: restriction policies, effective permissions, explain (GOV-006, GOV-007, ADR-0086; maintainer 2026-10-21) | E1e-a running (session 3); E1e-b next | `agents/briefs/next/E1e-a-restrictions-core.md`, `E1e-b-restrictions-api.md` |
| 18 | Merge `origin/main` (#37, #39, #40) into `feat/phase-9` | done (session 3, `f3759977`) | README §7 |
| 19 | CI jobs for IaC/air-gap; re-run Dart suites | done (session 3, `330624f4`; Dart suites green, one flaky wait fixed) | README §7, §9 |
| 20 | E12: gates, statuses, coverage floors, P9 PR | lead (last) | README §7 |

## Next steps for the new session, in order

1. Session protocol (`AGENTS.md` §2), then README §6 "Session start" (install the agent
   definitions, fill the `Claude-Session` line in `agents/prefix.md`).
2. Launch rows 3, 7, 8, 9, 12 together (five agents; types in README §5).
3. Review, fix and merge each as it finishes (README §6); then row 4; then 15, 16, 17.
4. Rows 18–20 (lead).
5. Keep `docs/WORKLOG.md` and this list current; report progress as a percentage table.
