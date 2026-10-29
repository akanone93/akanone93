<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Manual checks

A guide for the maintainer to check Plux by hand, live, end to end. It is not a phase and not
a test suite: every check is something you do on a running system, see with your own eyes and
record. The automated tests prove the claims; these checks are how you come to know the
system.

## The three layers

| Layer | What it covers | When | Where |
|---|---|---|---|
| **1. WIP requirements** | One check per requirement still `WIP` in the [specification](../requirements.md): what it needs, how to verify it by hand, and what moves it to `DONE`. | Now | [layer-1-wip/](layer-1-wip/README.md) |
| **2. Functional** | High-level checks of every feature as a user, an operator and a developer meets it: a guided tour of the whole system that builds one app from nothing to a device. | Now | [layer-2-functional/](layer-2-functional/README.md) |
| **3. Confidence** | Thousands of detailed checks, scenario by scenario and implementation by implementation (UI, API, logs, keys, workers, packages, canvas…), after which you know the system. | After the AI (P12), USSD and mini-app (P16) phases | [layer-3-confidence/](layer-3-confidence/README.md) |

Run layer 1 first (its results move rows to `DONE`), then layer 2 from start to end.

## Before you start

[00-setup.md](00-setup.md) brings up the stack, the device and the named resources the checks
reuse (`R-ADMIN`, `R-ORG`, `R-APP`, …). A check lists the resources it needs; a resource is
made once, by setup or by the check that first creates it, and later checks build on it.

## How a check is written

Every check uses the same short form, so a check reads in under a minute:

```markdown
### L2-STU-012 — Rename a screen and see the route follow

- **Needs:** R-ADMIN, R-APP, R-PLUGIN (from L2-STU-010)
- **Do:**
  1. In Studio, open R-PLUGIN, select the screen `home`, rename it to `start`.
  2. Open the Problems tab.
- **Expect:** the route changes to `/start`; deep links and generated API names that used it
  are listed; no problem remains.
- **Why:** routes are addresses other plugins and the host use (ROU-003).
- **Look at:** the server log line `document.change` with the new route.
- **Result:** ☐ pass ☐ fail — notes:
```

- **ID**: `L<layer>-<AREA>-<nnn>`. Areas: `SRV` server and API, `CMP` compiler and schema,
  `RUN` runtime, `SYN` sync and delivery, `SEC` security and keys, `FN` functions, `STU` Studio,
  `DEV` dev app and debugging, `OPS` operations and deployment, `L10N` localisation and
  accessibility, `GOV` enterprise and governance, `CLI` command line, `DOC` documentation.
- **Needs**: resources and earlier checks. Prefer what an earlier check left behind over new
  set-up.
- **Do**: numbered steps with exact commands, URLs and clicks.
- **Expect**: what you see when it works, observable without reading code.
- **Why**: one line, with the requirement IDs it touches.
- **Look at** (optional): the log line, metric, header, database row or file that shows it
  happened, so each check also teaches where to look.
- **Result**: filled in by you. A failure gets an issue and its link here.

Layer 1 checks also say **Moves to DONE when:** the evidence that closes the requirement.

## Working through the checks

- Copy nothing: tick the boxes in place on a branch (`checks/<date>`) and commit the results;
  the diff is the record.
- Stop at the first failure of a check that later checks depend on, and note it.
- Ask Claude in a session for help with any check: name its ID.
