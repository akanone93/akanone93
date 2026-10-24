<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0100. P11 changes to MUST requirements in specification 1.10.0

- **Status:** **Accepted** (maintainer, 2026-10-24; P11 plan M-9)
- **Date:** 2026-10-24
- **Requirements:** `STU-010`, `STU-024`, `STU-055`, `STU-060`, `STU-115`, Appendix J, §21 area table

## Context and problem

Planning P11 found places where the specification's `MUST` rows disagree with the accepted Studio
design or with what the server can safely offer. A change to a `MUST` requires an ADR (Document
Control), and the change lands with the code and every other document that restates it (AGENTS §3).
This ADR records the changes that have no home in another ADR. The wording changes for `STU-001`,
`STU-002`, `STU-006`, `GOV-002`, `GOV-003`, `CI-006`, `QA-007` and the technology table are recorded
in ADR-0013, ADR-0014, and the Revisions of [ADR-0026](0026-identity-tenancy-and-access.md) and
[ADR-0073](0073-roles-permissions-and-scoping.md). The list is §4.1 of
[the P11 plan](../plans/p11.md); the decisions are O-5, O-12, O-13, O-14 and F24.

## Decision drivers

- The specification and the product must say the same thing (AGENTS §3).
- Scope stays inside the phase: no placeholder for a later phase's feature (AGENTS §5).
- Security properties decided earlier are not weakened by a UI requirement.

## Considered options

For each change the plan weighed rewording the specification against changing the design or the
server. The maintainer chose to reword the specification in every case below; the alternatives are
named with each.

## Decision

Specification 1.10.0 changes as follows.

| Where | New wording | Alternative not taken |
|---|---|---|
| `STU-024`, `STU-055`, `STU-060` | The **AI entry points** (create an app or a screen from a prompt, the AI assistant in the action bar) **arrive with P12** (F24). P11 has no placeholder for them. | Build stubs now: rejected, a half-built later-phase feature. |
| §21 area table | Functions: "Go editor" becomes **"Rust editor"**. | None; functions are Rust (`FN-001`, `FN-033`). |
| `STU-115` | Studio **shows** signing keys and certificate pins with their rotation state and links the key-ceremony runbook; it **manages** capability grants (documents) and the security profile. | Online key and pin management: rejected, it changes the offline root design of [ADR-0004](0004-tuf-style-update-security.md) (O-14). |
| `STU-010` | "Generously rounded corners: 18 to 24 px on panels, cards and dialogs, full pills on controls; smaller radii only for small elements" (O-12). | Change the accepted design to 12 to 20 px. |
| Appendix J | Brand tokens are the wordmark and brand accents; Studio's interface uses the derived, contrast-measured scales of the UI language; arrows are navy at rest and violet when hovered or selected (O-13). | Keep the old use column: its text colours fail 4.5:1 on white. |

- **`STU-115`, reasons.** Signing keys are per environment and never chosen by a caller; pins live
  in the root metadata made by the offline key ceremony (`SEC-041`, `SEC-050`); no RPC exists for
  either. Studio therefore reads and explains them. Capability grants and the security profile are
  documents with existing services and stay manageable. The offline root keys stay offline.
- **`STU-010` and Appendix J** are design wording; their reasoning (contrast and the radius scale)
  is in ADR-0014, and the numbers here are the wording the specification
  takes.
- **Process.** `make spec-lint` passes after each edit. The `Status` of the rows is unchanged by
  this ADR; each stays `SPEC` until its slice lands.

## Consequences

- **Positive:** the specification matches the accepted design; P11 is not delivering stubs; the
  offline root-key design is intact.
- **Negative:** a reader of version 1.9.0 will find different words; `STU-024`, `STU-055` and
  `STU-060` are only partly delivered until P12, which the phase table must show.
- **Follow-up:** the specification edit lands in S0.0 with this ADR; the phase table and traceability
  for the AI entry points move to P12.
