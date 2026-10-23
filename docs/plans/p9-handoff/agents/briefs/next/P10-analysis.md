<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: analyse Phase 10 (Dev app and live debugging) and write `docs/plans/p10-analysis.md`

Documentation only (the exception to rule 1); no code. Read: spec §5 row P10 (`grep -n "P10" docs/requirements.md`), every requirement whose phase is `P10` (`grep -n "| P10 |" docs/requirements.md`) and every earlier-phase row whose text or status names P10 (`grep -n "P10" docs/requirements.md`), spec §1.4, §6.7 (layering) and the sections those rows cite; the plan format of `docs/plans/p9.md` (headings only); `docs/adr/README.md` for ADRs that touch the dev app, devtools, debugging, mocks, network simulation or incremental compilation; and the existing code: `packages/plux_devtools`, `apps/` (any dev or starter app), `backend/internal/compiler` (incremental compile), `backend/cmd/plux` (dev, watch, mock, viewer commands), `backend/internal/netsim`, the runtime's debug hooks in `packages/plux_flutter/lib` (grep `kDebugMode`, `devtools`, `inspect`, `trace`). Use grep and ranges; never dump large files.
Write the analysis with these sections:
1. **Scope:** the P10 goal and exit criteria verbatim from §5; every P10 requirement (ID, MUST/SHOULD, one-line summary).
2. **Current state per requirement:** what already exists in code (file paths), what is partial, what is absent — be exact, cite files.
3. **Gaps and work items:** grouped into candidate milestones (like P9's E0…E12), each with the requirement IDs it closes, the components touched (Go, Dart, CLI, apps), dependencies between milestones and a rough size (S/M/L).
4. **Conflicts and decisions for the maintainer:** any requirement that is ambiguous, contradicts another requirement or an ADR, or depends on a later phase (P11 Studio, P12 AI); each with options and a recommendation (AGENTS.md §3 format, short).
5. **Risks:** performance targets (`DEV-010` edit-to-device latency and any other budgets), security (debug features absent from release builds, pairing, live push channel), platform constraints (iOS/Android), CI needs (device jobs, new gates).
6. **Carry-overs from P9:** read `docs/WORKLOG.md` latest note and `docs/plans/p9-handoff/README.md` §9–§10 for items that land in P10.
Keep it factual and compact (about 250–400 lines). Report in at most 8 lines: the file, the milestone count, and the top three decisions needed.
