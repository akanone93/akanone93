<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E11a — runbooks, guides, threat-model delta and compliance pack (`DX-002` P9 parts, DoD-6, DoD-9)

Documentation only (this brief is the exception to rule 1's "no docs"). Model: you are Haiku — keep to facts you read in the code and the plan; never invent behaviour, flags or numbers.

Read: `docs/plans/p9.md` lines 673–684 (§5.14) and §5.12 (lines 643–665); the existing guide and runbook formats (`docs/guides/security.md` head, `docs/runbooks/README.md`, one runbook such as `docs/runbooks/key-compromise.md`); the threat model (`grep -rln "threat model" docs | head`); the generated references you link to (`docs/reference/permissions.md`, `docs/reference/cli.md`, `docs/operations/egress.md`, `docs/operations/personal-data.md`, `docs/operations/sizing.md` if present, `docs/operations/air-gapped.md`, `docs/runbooks/backup-and-restore.md`, `docs/runbooks/alerts/README.md`). For each feature, read the package `doc.go` of its code (`backend/internal/{approval,rollout,experiment,analytics,notify,auditexport,reconstruct,dsr,search,gitsync,push}`, `backend/internal/auth` federation and step-up files, `ee/backend/*`) — that is your source.

Deliver:
1. **Runbooks** (`docs/runbooks/`, the existing format: symptom, impact, diagnosis, remediation, escalation): approval deadlock and break-glass, stuck rollout, control-document key rotation, health-gate false positives, telemetry backlog, ClickHouse switch-over, SIEM backlog, data-subject request, licence expiry, restore, upgrade rollback, air-gapped upgrade; link each from `docs/runbooks/README.md`.
2. **Guides** (`docs/guides/`): governance (roles, permission catalogue, scoped grants, custom roles, approvals and policies, break-glass, freezes), SSO and SCIM, sessions and step-up (including the CLI flow), rollouts and experiments (bucketing, targeting, health gates, flags, experiments, statistics and their interpretation, promotion), analytics (ingestion, retention with the off-peak window, exports, dashboards, alerts, error grouping), operations (Helm, HA, backups and the drill, air-gapped, IaC, sizing, observability and SLOs, the egress list), notifications and webhooks.
3. **Threat-model delta** (DoD-6) in the existing threat-model document: federation (SAML wrapping, replay, SCIM token theft), approvals (content-hash binding, author self-approval, break-glass abuse), the online control role, telemetry personal data, exports (SIEM, webhooks, Git), the licence (tampering, offline verification).
4. **Compliance pack** (`docs/compliance/`, `DX-002` P9 part): an index linking the evidence (audit chain verification, compliance reports, reconstruction, data-subject requests, egress list, ASVS-style evidence pointers), and leave `docs/compliance/walkthrough.md` to the walkthrough agent.
Run `make reuse-lint` and the repository's Markdown/link checks if a Make target exists (`make help | grep -i -E "doc|link|md"`).
