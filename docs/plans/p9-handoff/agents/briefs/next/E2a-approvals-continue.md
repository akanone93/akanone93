<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: finish P9 E2a — the approval engine core. Wip branch: `wip/p9-e2a-approvals`

A colleague did most of this task and stopped on a usage limit while "writing the CLI test"; its work is saved as one commit on the wip branch (based on an older `feat/phase-9`, so the merge in rule 0 will have conflicts in the shared wiring files — resolve them by keeping both sides). Your full original task is `docs/plans/p9-handoff/agents/briefs/in-progress/E2a-approvals.md` — read it, then see what exists (`git diff --stat feat/phase-9...HEAD` after the merge), finish only what is missing, and make its done-check pass. Since the colleague started, the E5 server merged an `ApprovalGate` seam in `backend/internal/experiment` (grep it): implement it here for ABT-007 (experiments touching secure pages or money flows under `maximum` need approval). Commit at each working step.
