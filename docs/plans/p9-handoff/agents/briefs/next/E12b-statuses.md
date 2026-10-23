<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E12b — set the P9 requirement statuses (spec §4.3, `QA-070`, `QA-071`)

This brief is the exception to rule 1: you edit only the `Status` column of `docs/requirements.md` for rows whose phase is `P9` (`grep -n "| P9 |" docs/requirements.md`), and of rows from earlier phases that P9 completed (the plan `docs/plans/p9.md` §3 lists what P9 carries). Nothing else in the file changes.
Rules (spec §4.3; read its definitions with `grep -n "^### 4.3" -A25 docs/requirements.md`): `DONE` only when the behaviour is implemented on `feat/phase-9` **and** at least one automated test or CI check names the ID (Go `_GOV_005`/`// Verifies:`, Dart `[GOV-005]`, Makefile/workflow `# Verifies:`) — confirm each with `grep -rn "<ID>" backend ee tools packages apps mk .github test --include=*.go --include=*.dart --include=*.mk --include=*.yml --include=*.sh | head -3`; `WIP` when partly implemented or verified only by a check that cannot run yet (the kind walkthrough, the restore drill and the air-gapped install have never run: their IDs are `WIP` unless another test names them); keep `SPEC` when nothing exists. Never mark `DONE` from a document alone.
Then run `make spec-lint` and `make trace` and fix your own statuses until both pass (0 violations). Report the counts per status and every ID you left `WIP`, with the reason, in at most 8 lines.
Done when: `make spec-lint && make trace` pass.
