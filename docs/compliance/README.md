# Compliance mappings

Mappings from regulations and standards to Plux requirements (spec §31): GDPR, other national data-protection laws, DORA, the European Accessibility Act, PCI DSS and financial-regulator directives. Each mapping is written when the capabilities it maps exist, starting in **P6**.

## Evidence index (P9)

This index says where an auditor finds the evidence for each control the P9 governance and
operations work provides (`DX-002`). Each row names the requirement, the place the evidence lives
and how to check it. The requirement statuses are in the `Status` column of
[the specification](../requirements.md), not here.

| Control | Requirement | Where the evidence is | How to check it |
|---|---|---|---|
| Audit chain is intact and append-only | `SEC-140` | The audit log of each organisation; the checkpoints listed by `SecurityAdminService.ListAuditCheckpoints`; alert [PluxAuditChainBroken](../runbooks/alerts/PluxAuditChainBroken.md). | The restore drill verifies the chain (`make restore-drill`, [operations guide](../guides/operations.md#3-backups-and-the-restore-drill)). A broken chain raises the alert. |
| Approvals are signed, and bound to the exact artifact | `GOV-020` to `GOV-023` | Approval records in the audit chain, signed by the audit signing role, listed by `ApprovalService.ListApprovals`. | Each record verifies offline against the audit key. [Governance guide](../guides/governance.md#8-approvals-and-policies). |
| Who may do what, and why | `GOV-002`, `GOV-003`, `GOV-006`, `GOV-007` | Grants, roles, restrictions, and the permission catalogue in [the permissions reference](../reference/permissions.md). | `plux access explain` and `GovernanceService.ExplainAccess` give the reason for one decision. [Governance guide](../guides/governance.md). |
| Break-glass is used with justification and reviewed | `GOV-024` | `BreakGlassService.ListBreakGlass`, with the review that follows each use. | [Governance guide](../guides/governance.md#9-break-glass). |
| Production changes and their approvers, for a period | `GOV-026` (enterprise) | Signed compliance reports: `ComplianceService.ListComplianceReports` and `GetComplianceReport`. | Verify the report's signature with the verifier named in [ADR-0076](../adr/0076-approval-engine.md). |
| Sign-in strength and session control | `GOV-004`, `SEC-100`, `SEC-103` | Sessions (`IdentityService.ListSessions`), step-up assertions, and federation settings. | [Sessions guide](../guides/sessions-and-step-up.md) and [SSO guide](../guides/sso-and-scim.md). |
| Audit export to a SIEM | `SEC-143` | Destinations (`AuditService.ListExportDestinations`) and their cursors. Each record carries its entry hash and the previous one. | A SIEM that verifies the chain detects any gap. [ADR-0082](../adr/0082-audit-export-reconstruction-and-egress.md). |
| What a user saw at a moment | `SEC-142` | Signed reconstruction releases (`AuditService.ListReconstructions`, `GetReconstruction`), viewed in the viewer host. Every view is audited. | The release verifies with the environment's root keys before the viewer parses it (`SEC-052`). |
| Data-subject requests | `SEC-162` | Requests and their tombstones (`DataSubjectService.ListRequests`, `GetRequest`); the personal-data map. | [Personal data map](../operations/personal-data.md). An erasure leaves tombstones that carry a hash of the subject, not the identifier. |
| Every outbound connection is listed | `SEC-163` | [The egress list](../operations/egress.md), generated from the code's registry. | CI checks the list against the code, so a new destination must appear in it. |
| Security controls against a standard | `SEC-*` | [ASVS evidence for the server](../security/asvs-server.md), [MASVS](../security/masvs.md), [threat model](../security/threat-model.md), [whitepaper](../security/whitepaper.md), [privacy](../security/privacy.md). | Every `MUST` requirement marked `DONE` names the test that verifies it (`QA-070`). |

Where a row names an enterprise feature, the core does not produce the evidence without the
enterprise edition's licence (`GOV-032`: security capabilities are never gated by edition, so
the audit log, reconstruction, data-subject requests and egress control are in the core).
