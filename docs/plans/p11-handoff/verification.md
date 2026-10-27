<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Verification at hand-off

Last results on the merged tree (`feat/phase-11`, 2026-10-26/27):

| Gate | Result |
|---|---|
| `make studio-check` | exit 0 — 2,947 tests, 0 fail; initial JS 290 KB gzip of 1.5 MB |
| `make studio-e2e` | exit 0 — 19 passed |
| `make studio-design-gate` | exit 0 (clean) |
| `make studio-conformance` | 0 fail (1,928 pass at the last counted run, before S4.6b added scenes) |
| `make dart-analyze` | no issues |
| `make dart-lock-check` | exit 0 |
| `make gen-check`, `make spec-lint` | exit 0 |
| `make go-lint` | 0 issues |
| `go test -race` | tenancy, document, api, compiler, pxl, importer, builder, builderrole, buildclient, config, audit: ok |
| `cd tools && go test ./...` | ok |
| `make studio-bench-canvas` | all M-12/M-13 and S3.2 targets met (docs/benchmarks/studio.md) |

**Not run in session 1:** `make check` (whole repository), CI (by decision M-6: once, at the end),
`make trace`, the Docker image build and `studio-image-repro`, `zap-studio`, the full Flutter suite.
The two WIP branches are not verified at all.
