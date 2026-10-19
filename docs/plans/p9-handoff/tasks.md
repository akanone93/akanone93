<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Task list snapshot (2026-10-18, 17:45 UTC)

The lead's task list at the hand-off, as the maintainer followed it. Status words:
`completed`, `in progress` (stopped mid-work; see README §4 and the patches), `pending`
(not started).

| # | Task | Status | Notes |
|---|---|---|---|
| 1 | L4a: locale data in the bundle | completed | P8 |
| 2 | Fix catalogue-check "no builder" | completed | P8 |
| 3 | L4b: runtime localisation | completed | P8 |
| 4 | Dart test fixes | completed | P8 |
| 5 | L5c: compiler l10n checks, pseudo-locales, report | completed | P8 |
| 6 | K3b-maps: MapView, OrderTrackerMap | completed | P8 |
| 7 | K3b-scan: scanners, QrCodeView, B21 | completed | P8 |
| 8 | Small batch: decisions to implement | completed | P8 |
| 9 | X1: cross-phase integration X1.1–X1.7 | completed | P8 |
| 10 | Wave 5: L8 goldens, L7 a11y passes, L9 reference apps | completed | P8 |
| 11 | Wave 6 / L10: docs, CI, version, statuses, gates | completed | P8 |
| 12 | Fix device e2e CI shards and re-run CI | completed | P8; the last iOS bank fix (`796ddbd`) still needs its green run |
| 13 | 40× race run of TestPublishWithTheCLIAndSyncADevice, e2e-starter, WORKLOG + report | completed | Both passed locally |
| 14 | P9 plan written; approvals A1–A6 | completed | `docs/plans/p9.md` |
| 15 | P8: CI on `796ddbd` (iOS doubled-key fix) → open PR to main | **in progress** | Run 39149009669; PR body `p8-pr-body.md` (README §2) |
| 16 | P9 E0: spec 1.7.0, ADRs 0072–0085, error codes, Q24 fixes | completed | Merged on `feat/phase-9` |
| 17 | P9 E1: roles, scoping, OIDC, SAML, SCIM, sessions, step-up | **in progress** | E1a and E1c patches; E1b (OIDC extension, SAML, SCIM) not started |
| 18 | P9 E2: approval engine, break-glass, freezes, compliance reports | pending | Needs E1 merged |
| 19 | P9 E3: signed control document, kill switch, mandatory update, silent push | **in progress** | E3 patch (barely started) |
| 20 | P9 E4+E5: rollouts, health gates, flags, experiments | pending | Needs E3, E6 |
| 21 | P9 E6+E7: telemetry/analytics, notifications, webhooks, triggers | **in progress** | E6a and E7 patches; E6b (ClickHouse, Parquet, dashboards, alerts, error grouping) not started |
| 22 | P9 E8: SIEM export, reconstruction + viewer, DSR, egress, search, Git export | pending | Needs E2, E3, E6 |
| 23 | P9 E9+E10: Helm/HA/backups/air-gap/observability/IaC; ee/ + licence | **in progress** | E9a done (§7 of README); E10 patch; E9b (backups, drill, air-gap, observability, SLOs, sizing, IaC) not started |
| 24 | P9 E11+E12: reference apps, walkthrough, docs, close gates | pending | Last |

## Next steps for the new session, in order

1. Check CI run 39149009669 (`796ddbd`); on green, open the P8 pull request (README §2).
2. On `feat/phase-9`, resume the seven stopped slices from their patches (README §4) —
   E1a and E10 first (others depend on `rbac_p9.go` and `backend/internal/edition`), then
   E6a, E7 (E9a is done, README §7), and restart E1c and E3 from their briefs (`briefs.md`, section B).
3. Merge each finished slice into `feat/phase-9` with `tools/merge.sh`, run `make gen`, the
   slice's checks and the pre-push `go test`, push.
4. Launch the next wave per plan §6.2: E1b, E2, E6b, E9b; then E4, E5, E8; then E11, E12.
5. Keep `docs/WORKLOG.md` and this task list current; report progress as a percentage table
   per milestone.
