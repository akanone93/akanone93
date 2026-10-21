<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E2b — break-glass, change freezes (ee), compliance reports (ee) (`GOV-024`, `GOV-025`, `GOV-026`)

Starts after E2a (the approval engine, `backend/internal/approval`, `approval.Require(ctx, artifact)`) is merged — build on it; never a second approval path.

Read: `docs/plans/p9.md` lines 432–447 (§5.3: Break-glass, Change freezes, Compliance reports, Wiring the stand-ins) and §2.6 (freezes and compliance reports are ee; break-glass is core); `docs/adr/0076-*.md`; `grep -n "GOV-02[4-6]" docs/requirements.md`. Codes `PLX-8005` (justification required), `PLX-8006` (post-hoc review overdue), `PLX-8010` (change freeze active); permissions `auth.BreakglassInvoke`, `auth.FreezesManage`, `auth.ComplianceReport`; edition features `FeatureChangeFreezes`, `FeatureComplianceReports` (`backend/edition`). The ee seams go through public packages (`backend/ext/<area>`), as `backend/ext/identity` and `backend/ext/export` do. Notify approvers through `notify.Publish` (event "break-glass used" exists in `backend/internal/notify/events.go`).

Deliver:
1. **Break-glass (core, GOV-024):** designated roles (the permission) run kill switch (`release` control document ops), rollback and function disable without approval, with a written justification (`PLX-8005` without); approvers alerted at once; a post-hoc review opened with a due time (configurable, default 24 h), escalated when overdue (`PLX-8006`, worker job); every step audited. Wire it into the existing kill-switch/rollback/function-disable handlers as an explicit `--break-glass --justification` path; ordinary calls still go through `approval.Require`.
2. **Change freezes (ee, GOV-025):** calendar windows per organisation and environment, one-off and recurring (RRULE subset: daily/weekly with BYDAY, or cron-like — choose and document), with time zones and DST; production changes refused with `PLX-8010` inside a window except through break-glass; public seam `backend/ext/governance` (`FreezeCalendar`), core default "no freezes", ee implementation gated with `FeatureChangeFreezes`.
3. **Compliance reports (ee, GOV-026):** for a period, every production change with author, approvers, timestamps, content hashes and evidence links; signed JSON (DSSE envelope, signed through the audit signing path — only `internal/signing` touches keys) and a PDF from an in-house writer (text, tables, a signatures page; no dependency — PDF 1.7 with the standard Helvetica font is enough); `plux compliance report --from --to --format json|pdf` and `plux compliance verify <file>`; gated with `FeatureComplianceReports`.
4. Migrations (next free numbers after the highest on `feat/phase-9`), `governance.proto` additions + `make gen`, handlers, matrix entries, CLI.
Tests (`_GOV_024`…`_GOV_026`): justification required, approvers notified, review overdue escalation with an injected clock, freeze windows across a DST change and time zones, freeze refused / break-glass allowed, report contents for a seeded period, DSSE signature verifies and a changed byte fails, the PDF parses (check structure: xref, trailer, page count), ee features refused without licence.
Done when: `cd backend && go build ./... && GOWORK=off go build ./... && go test -count=1 ./internal/approval/... ./internal/release/... ./internal/api/... ./cmd/plux/...` and `cd ee/backend && go test -count=1 ./...` pass.
