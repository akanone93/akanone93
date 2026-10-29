<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Cross-phase audit (parked)

**Status: parked (maintainer, 2026-10-29).** Scheduled after the last feature phases: P12 (AI
generation), the USSD phase (not yet in the specification) and P16 (super-app and partner
mini-apps). Before it, the [manual checks](../manual-checks/README.md) of layers 1 and 2 are
done; the audit then writes and works through layer 3.

## What

One pass over every requirement of P0 to the last phase, checking each against the code, its
tests and CI evidence rather than its `Status` column alone, together with the maintainer's
hands-on [layer 3 confidence checks](../manual-checks/layer-3-confidence/README.md).

## Aim

Everything that claims to work works, end to end, securely and within its budgets; every
`WIP` row becomes `DONE` with evidence, a concrete gap, or a maintainer decision. Evidence
work, not new features.

## Scope

1. **Status reconciliation.** Every row of every phase: evidence named, gap listed, or decision
   asked. Rows still `WIP` after the layer 1 checks first.
2. **End-to-end flows.** Studio → server → device for the main journeys (create, publish,
   rollout, kill switch, approvals), the audit walkthrough with Studio-driven steps (A-1), and
   the kind jobs with Studio (A-3).
3. **Security sweep.** Threat model, authorisation matrix, tenancy and row-level security,
   keys and secrets, CSP and headers; CodeQL, ZAP and dependency review across components.
4. **Performance.** Every budget re-measured in CI with margin: the canvas drop settle (A-5),
   database test speed (A-2).
5. **CI robustness.** Registry pulls (A-4); no check left ignored by default.
6. **Layer 3.** Write the confidence checks and work through them with the maintainer.

The collected items A-1 to A-5 are in [audit-phase-notes.md](audit-phase-notes.md).

## Milestones

| Milestone | Content |
|---|---|
| AU-0 | Status inventory and gap list for every phase |
| AU-1 | Security and correctness gaps fixed |
| AU-2 | End-to-end flows and the walkthrough |
| AU-3 | Performance and CI hardening |
| AU-4 | Layer 3 written |
| AU-5 | Layer 3 worked through with the maintainer; exit report; one CI run on the PR |

## Size

AU-0 to AU-3: about three to four sessions, agents per area with one gate per milestone; a
gap that turns out large becomes its own item instead of growing the audit. AU-4 and AU-5 are
as long as the maintainer's walk-through takes.
