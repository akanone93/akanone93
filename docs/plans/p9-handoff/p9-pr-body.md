<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# feat: Phase 9 — enterprise and operations

## Summary

P9 of `SRS-PLUX-001` (spec 1.8.0): roles, scoped grants, custom roles, restriction policies with effective permissions and explain (`GOV-002`–`GOV-007`, ADR-0073, ADR-0086); OIDC, SAML, SCIM, sessions and step-up; the approval engine, break-glass, change freezes and compliance reports (`GOV-020`–`GOV-026`); the signed control document with kill switches, mandatory updates and silent push; staged rollouts with health gates; flags and experiments with in-house statistics; telemetry, analytics, dashboards, alerts and error grouping; notifications, webhooks and triggers; SIEM export, reconstruction and its viewer, data-subject requests, the egress registry, search and Git export; the Helm chart with HA, backups and the restore drill, the air-gapped bundle, IaC and observability; the `ee/` edition with its licence; runtime `plux_flutter` 0.6.0; reference-app scenarios, runbooks, guides, the threat-model delta, the compliance pack and the scripted audit walkthrough.

## Incompatible changes (before 0.0.1)

- Proto messages renamed for buf lint: `ControlDocumentResponse` → one response per control RPC; `FlagResponse`, `ExperimentResponse`, `HoldoutResponse` → one per RPC; Git export `GetGitSettings*`/`ConfigureGitExport*`/`ListGitExports*`/`RunGitExport*` → `GetSettings*`/`ConfigureExport*`/`ListExports*`/`RunExport*`; `WebhookService.ListWebhookDeliveries` has its own messages.
- `plux_flutter` 0.5.0 → 0.6.0; dependents move to `^0.6.0`.
- Chart: `networkPolicy.externalHTTPS.enabled` defaults to `false` (SEC-163, maintainer 2026-10-21).
- Viewer: `PluxConfig.rootKeys` in reconstruction mode are pinned root-role keys.

## Requirements

52 of the 59 P9 rows are `DONE`; `GOV-020`, `ANL-040` wait for Studio (P11); `DEP-004`, `DEP-010` for their first kind run; `NFR-040` has no check yet.

## Verification

- `make -k check`: every gate passes except `go-vuln` (vuln.go.dev is unreachable in the agent environment; CI runs it).
- `make trace`: 0 violations. `make spec-lint`: no problems.
- Not run here: kind, Docker and OpenTofu init (restore drill, air-gapped install, walkthrough, IaC); `make e2e-starter`; the 40 race runs; the dispatched CI run.

## Open

- The walkthrough (`make walkthrough-kind`) needs a WebAuthn step-up driver and an enterprise image with a licence before CI can run it; no CI job runs it yet.
- Limit defaults (hand-off 2 §9 item 6, plus `auth.restrictions*`, `auth.explainPerMinute`, `auth.simulatePerMinute`), the ADR-0066 locale rule and the legal review of `ee/LICENSE`.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

https://claude.ai/code/session_01JKK1NrZX8s8Ewri18tJK82
