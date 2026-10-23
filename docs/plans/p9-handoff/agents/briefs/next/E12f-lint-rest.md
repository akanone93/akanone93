<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E12f — make `make go-lint` pass

`make go-lint` (run from the repository root) reports 13 findings on `feat/phase-9` (log: `/tmp/claude-0/-home-user-plux3/48b3b72a-2abb-5762-a1af-a581b93bd4ab/scratchpad/lint2.log`; grep `\.go:[0-9]+:[0-9]+: `), and the linters' per-message caps may hide a few more. Fix each at its cause, following `docs/engineering/go-standards.md`: G101 on constants that are names, not secrets — rename so the identifier does not read as a credential, or move the value; G703 — confine the path (`os.Root`, as `backend/cmd/plux/userfiles.go` does); contextcheck — pass the context; wrapcheck — wrap; unused receivers — `_`. **Never add `//nolint`** and never change `.golangci.yml`. Behaviour must not change. Repeat `make go-lint` until it passes.
Done when: `make go-lint` passes, and `go build ./... && go vet ./...` plus the tests of every package you touched pass.
