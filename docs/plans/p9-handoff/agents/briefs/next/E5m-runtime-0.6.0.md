<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E5m — `plux_flutter` becomes 0.6.0, the P9 runtime (`RT-001`, `WGT-004`, `BND-008`)

The registry already says P9's `Variant` node shipped in runtime 0.6.0, but the runtime still reports 0.5.0 (`packages/plux_flutter/lib/src/runtime_info.dart`), so a device would refuse a bundle that uses it (BND-008). Mechanical work only:
1. `plux_flutter` 0.6.0: `pubspec.yaml`, `runtime_info.dart`, a `## 0.6.0` CHANGELOG entry ("The runtime of Plux Phase 9: …" — list the P9 runtime features from `git log --oneline main..feat/phase-9 -- packages/plux_flutter/lib | head -60`, one line each, in the style of the 0.5.0 entry).
2. Every dependent `plux_flutter: ^0.5.0` → `^0.6.0` (`grep -rln "plux_flutter: \^0.5.0" apps packages`), then `flutter pub get` in each so the lock files follow.
3. Generators that emit `0.5.0` for the runtime (`backend/internal/generator/generator.go`, `backend/internal/codegen/packages.go`, `backend/internal/plxtest/harness.go` — check each hit is the runtime version, not something else) and their goldens with the repo's `-update` flags.
4. `schema/testdata/documents/plux_express/app.json` `minRuntimeVersion` → `0.6.0` (it uses `Variant`), then remove `--acknowledge-warnings` and its comment from `backend/internal/server/reference_e2e_integration_test.go`; update the Express goldens with `-update`.
Do not change other apps' `minRuntimeVersion`, and do not touch `backend/internal/schema/registry` (generated).
Done when: `cd backend && go test -count=1 ./internal/generator/... ./internal/codegen/... ./internal/plxtest/... ./cmd/plux/ && go test -count=1 -run 'Express|Bank' ./internal/server/` and `cd packages/plux_flutter && dart analyze lib test` pass.
