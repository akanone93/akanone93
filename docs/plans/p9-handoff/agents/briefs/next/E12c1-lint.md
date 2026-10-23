<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E12c-1 — fix the golangci-lint findings in your half

`make go-lint` reports 155 findings on `feat/phase-9`; yours are the lines of `/tmp/claude-0/-home-user-plux3/48b3b72a-2abb-5762-a1af-a581b93bd4ab/scratchpad/lint-part-1.txt` (path:line:col: message (linter)). Fix each at its cause, following `docs/engineering/go-standards.md` (rule 3): split long or complex functions, wrap errors, pass contexts, use `http.NewRequestWithContext`, fix gosec findings properly (never weaken a check). **Never add `//nolint`** and never change `.golangci.yml`. Do not touch files outside your list except to keep the build green. Behaviour must not change.
Done when: `cd backend && go build ./... && go vet ./...`, the tests of every package you touched pass (`go test -count=1 ./<pkg>/`), and `golangci-lint run ./<each touched package>/...` (from its module root) reports none of your findings.
