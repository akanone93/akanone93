<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E12d — fix the 44 buf lint findings in the P9 protos

`make proto-lint` fails with the findings in `/tmp/claude-0/-home-user-plux3/48b3b72a-2abb-5762-a1af-a581b93bd4ab/scratchpad/proto-findings.txt` (`control.proto`, `experiment.proto`, `gitexport.proto`, `notification.proto`; mostly request/response names, e.g. `ListDeliveriesResponse` → `ListWebhookDeliveriesResponse`). These APIs are new in P9 and no release (0.0.1) exists, so renames are allowed; list every renamed message in your report (the PR calls out incompatible changes). Rename in the protos, run `make gen` (`PATH=$PATH:/root/.cache/plux/flatc-25.9.23`), then fix every Go and Dart caller (`grep -rn` the old names in backend, ee, packages, apps, studio). Never edit generated files by hand; never add buf lint exceptions.
Done when: `make proto-lint` passes, `cd backend && go build ./... && GOWORK=off go build ./... && go test -count=1 ./internal/api/ ./cmd/plux/`, `cd ee/backend && go build ./...` and `cd packages/plux_flutter && dart analyze lib test` pass.
