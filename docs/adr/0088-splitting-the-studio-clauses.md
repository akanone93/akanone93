<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0088. Splitting the Studio clauses of the dev-session requirements

- **Status:** Accepted (maintainer, 2026-10-22; P10 plan D-3, D-4)
- **Date:** 2026-10-22
- **Requirements:** `DEV-002`, `DEV-010`, `DEV-011`, `DEV-021`, `DEV-030`, `DEV-031`, `DEV-032`, `DEV-033`, `NFR-030`, `STU-130`, `STU-131`, `STU-132`, `STU-133`, `STU-134`

## Context and problem

Several P10 rows are written from Studio's side: "Studio MUST be able to open any screen"
(`DEV-011`), "select the corresponding node in Studio" (`DEV-030`), "from Studio autosave"
(`DEV-010`). Studio is P11. The P10 exit criterion needs only a Studio-less path, the CLI viewer,
and the device, relay and CLI deliver everything a Studio interaction will later call. Left as
they are, these rows could not be `DONE` at the P10 exit, and a test that plays Studio would
claim a requirement it does not meet. The Document Control rule says a change to a `MUST`
requirement needs an ADR. The question: how are the rows split so that `make trace` stays honest?

## Decision drivers

- A `DONE` status must mean what the row says (`QA-070`, `QA-071`).
- The device, server and CLI capability is a real deliverable of P10; the Studio interaction is a real deliverable of P11.
- Precedent: `FN-031` and `FN-032` were split with `FN-034` to `FN-036`; P9 left `ANL-040` `WIP` for its Studio half.
- Properties of the channel itself are not Studio features.

## Considered options

1. Split the rows: the capability stays in the `DEV-` row, the Studio interaction becomes a new P11 `STU-` row.
2. Keep the rows whole and `WIP` until P11, with the split recorded in the plan (the `ANL-040` precedent).
3. Move the whole rows to P11 and leave P10 without requirements.

## Decision

Chosen option: **1**, because it makes the statuses true on both sides at the cost of one
specification edit now.

- **`DEV-002`** binds the pairing token to "the user who creates the session", in Studio or with
  the CLI. No new row: the QR is shown by whichever client created the session.
- **`DEV-011`, `DEV-030`, `DEV-031`, `DEV-032`, `DEV-033`** state the capability of a *paired
  client* (the CLI in P10): the device carries out the command or reports the node, the relay
  authorises it. Each gets a new P11 row for the Studio interaction, in the new §21.13:
  `STU-130` (open a screen), `STU-131` (inspect in both directions), `STU-132` (state, mocks,
  network, clock, lifecycle), `STU-133` (PXL console, `SHOULD`), `STU-134` (screenshots into
  Studio, `SHOULD`). Priorities carry over.
- **`DEV-010` and `DEV-021` stay whole.** The latency and the trace file are properties of the
  channel. `DEV-010` and `NFR-030` get the P10 measurement method (D-4): from a file save in
  `plux dev` to the section applied and rendered on the device, over the netsim good-Wi-Fi
  profile, with the server part reported separately; Studio's autosave and render budget join
  the measurement in P11.
- `DEV-001` also changes (`plux_security` in a development mode, ADR-0091) and the Appendix K
  counts are recounted; neither is a split.

## Consequences

- **Positive:** the P10 rows can reach `DONE` on the CLI path; the P11 rows carry the Studio
  work with their own tests; `make trace` shows which half is verified.
- **Negative:** five more P11 rows (three `MUST`, two `SHOULD`); a reader has to follow a
  cross-reference between the halves.
- **Follow-up:** Studio's P11 plan takes `STU-130`–`STU-134` and the bottom drawer (`STU-066`).
  `DEV-020` and `DEV-021` describe the stream and are not split.

## Options in detail

### Option 1 — split

Described above. More specification work now; honest statuses afterwards.

### Option 2 — keep whole, `WIP`

No spec change, but `DEV-011` and the rest stay `WIP` through the whole phase although everything
but the Studio button exists, and the status says little. Rejected for the rows that name a
Studio-only interaction; it is exactly what was kept for `DEV-010` and `DEV-021`, whose text is
about the channel.

### Option 3 — move to P11

Leaves P10 with a relay nobody can ask to do anything and no requirement to verify it against.
Rejected.
