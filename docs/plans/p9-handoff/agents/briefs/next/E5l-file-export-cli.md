<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E5l — `plux analytics files show|set` for the enterprise file exports (`ANL-011`)

The RPCs exist: `AnalyticsService.GetFileExport` and `SetFileExport` (`proto/plux/v1/analytics.proto`, messages `FileExport`, `FileExportSettings`, `SetFileExportRequest.webhook_secret`). The CLI has the same shape for the OTLP export: `backend/cmd/plux/cmd_analytics.go` (`case "export":`, `analyticsExport`, `exportView`, `exportLine`) and its test `cmd_analytics_test.go` (`fakeAnalytics`, `runSub("export", …)`).
Do only: add `plux analytics files show` and `plux analytics files set` (flags for every `FileExportSettings` field; `--webhook-secret-file <path>` reads the secret from a file or `-` for stdin — never a flag value, so it stays out of shell history), copying the export subcommand's structure, output and `--json` handling; the completion entries (`backend/cmd/plux/completion.go`); `fakeAnalytics` gains the two methods. Never print the secret; show `secret_hint`.
Tests `_ANL_011`: show prints settings, hint and last error; set sends the fields and the secret read from the file; a missing secret file fails cleanly.
Done when: `cd backend && go test -count=1 ./cmd/plux/ && go vet ./cmd/plux/` pass.
