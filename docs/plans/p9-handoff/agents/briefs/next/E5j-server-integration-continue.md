<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: finish P9 E5j — server integration. Wip branch: `wip/p9-e5j-integration`

A colleague did items 1, 2, 3, 5 and 6 of `docs/plans/p9-handoff/agents/briefs/next/E5j-server-integration.md` (read it) and stopped in item 4 (enterprise export scheduling: `backend/ext/analytics/export.go`, `backend/internal/analytics/file_export.go`, `postgres_events.go`, migration `0045_analytics_file_exports.sql`, an egress destination, limits — uncommitted work saved in the last wip commit). Since then E5i merged on `feat/phase-9` and rewrote `packages/plux_flutter/lib/src/experiments/` (models named after `schema/control.fbs`): the rule-0 merge will conflict there — keep E5i's models and re-apply the colleague's bundle-defaults reading on top. Migrations: renumber to the next free after `feat/phase-9`'s highest.
Do only:
A. Finish item 4 and its tests.
B. **Every variant's props, not only the control's:** `AllocationVariant` in the control document carries no prop values, so a device assigned a treatment has no value for `experiment.<key>.<prop>`. Extend the bundle's `ExperimentDefaults`/`ExperimentProp` (`schema/fbs/bundle.fbs`, additive) so the compiler embeds each variant's value of each declared prop (deterministic order: the experiment document's variant order), and make the runtime read the assigned variant's value, falling back to the control's. Remove `ControlVariant.props` if nothing fills it then. One Go and one Dart test.
Done when: the done-check of the original brief passes.
