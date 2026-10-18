# 0072. P9 scope: Phase P16 for mini-apps, `SEC-191` to P12, `DX-005` and `DEP-060` withdrawn

- **Status:** Accepted
- **Date:** 2026-10-18
- **Requirements:** `HST-020`, `SEC-191`, `DX-005`, `DEP-060`

## Context and problem

Phase P9 ("Enterprise and operations") carried four requirements that do not belong in it, or
that the maintainer no longer wants. `HST-020` (mini-app mode) is a product of its own: a
super-app needs isolation, partner accounts with a permission model, a host review and
publishing flow, and possibly web-based mini-apps. `SEC-191` (independent penetration test) is
an external engagement that cannot be done inside the P9 build. `DX-005` (VS Code extension)
and `DEP-060` (public demonstration environment) are not worth their cost. The P9 plan (§2.1
Q1, Q3, Q4; §2.5) records the maintainer's decisions of 2026-10-18; this ADR records their
effect on the specification (1.7.0).

## Decision drivers

- Every phase ends in an independently demonstrable increment (spec §5); P9 must be closable.
- A requirement is never deleted; numbers are never reused (spec §4.1, `QA-073`).
- Security work scheduled "before Plux 1.0" must sit in a phase that precedes 1.0 (P12).
- `SEC-054`: no downloaded executable code except PXL bytecode, action graphs and sandboxed
  WebAssembly.

## Considered options

1. Keep all four in P9 and let the phase grow.
2. Move or withdraw each with a recorded rationale (chosen).
3. Delete the requirements.

## Decision

Chosen option: **2**.

- **A new phase P16, "Super-app and partner mini-apps"**, in milestone M4 after P15, whose
  tenancy work it builds on. `HST-020` is retagged P16. The idea in two sentences: a host
  organisation runs several Plux apps and web-based mini-apps inside one binary, each isolated
  with its own bundles, state, storage, theme and host-approved capabilities, and deeply
  integrated with the host's plugin system. The host can also invite partner organisations as
  accounts with a detailed permission model, which build and maintain their own (native-widget
  or web-based) mini-apps, submit them for the host's review, and publish into the super-app
  only after the host approves.
- **Exit criterion of P16:** a partner organisation builds, submits and, after the host's
  review, publishes a mini-app into a host super-app, isolated from the host's and other
  mini-apps' state, storage and events.
- **P16 is planned in its own plan** with its own requirement IDs. **Web mini-apps need an ADR
  against `SEC-054` before P16 starts any web runtime work**; until that ADR is accepted, only
  native-widget mini-apps may be planned for implementation.
- **`SEC-191`** is retagged P12 ("before Plux 1.0"). Nothing is done for it in P9.
- **`DX-005`** and **`DEP-060`** are `WITHDRAWN` (maintainer, 2026-10-18), with their rows
  kept for traceability. The maintainer deploys the product for its market, so no public
  demonstration environment is built.

## Consequences

- **Positive:** P9 can be closed; mini-app isolation gets a design of its own; the penetration
  test sits where it can gate 1.0; two low-value items stop costing effort.
- **Negative:** the "Super-apps" entry in the product context and `HST-020` stay unmet until
  P16, the last phase; the penetration test is not available as P9 evidence.
- **Follow-up:** the P16 plan and its requirement IDs; the web mini-app ADR against `SEC-054`;
  the P12 plan schedules `SEC-191`.

## Options in detail

### Option 1: keep everything in P9

Simple bookkeeping, but P9 would carry a phase-sized product and an external engagement, and
its exit could not be demonstrated by the automated walkthrough of the plan (§5.12).

### Option 2: move or withdraw with a rationale

Keeps traceability (`QA-073`), keeps each item in the phase that can deliver it, and costs only
a specification revision.

### Option 3: delete

Breaks the rule that identifiers are immutable and never reused (spec §4.1). Rejected.
