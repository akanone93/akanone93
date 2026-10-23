<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 integration (Dart runtime) — connect the merged server contracts on the device (`ABT-002`, `ABT-006`, `REL-051`, `SEC-142`, `REL-070`)

Several server slices merged with device seams left open. Dart in `packages/plux_flutter` (read `packages/AGENTS.md` headings); regenerate Dart protobuf/FlatBuffers only through `make gen`.

1. **Flags and experiments from the control document.** The E5 runtime built plain Dart models (`lib/src/experiments/`, carried in `ControlSnapshot`) because `schema/control.fbs`'s `FlagValue`, `ExperimentAllocation`, `TargetingRule` tables (and `ControlDocument.holdout`) were empty then; the E5 server has now filled them (read their schema comments — they define the assignment procedure: bucket = first 8 bytes of SHA-256(salt ‖ subject), big-endian, mod 10000). Make `lib/src/control/control_reader.dart` (it carries a hand-written verifier layout, see 3) fill those models from the verified document; delete any JSON-only path that exists only because the tables were empty; keep the models' field names in line with the schema. Test with documents built by the generated Dart builder.
2. **Device targeting for rollouts (REL-051).** The E4 server reads `GetManifestRequest.device_targeting` (`proto/plux/v1/manifest.proto`; hashed user attributes only, scheme `HashUserAttribute` in `backend/internal/rollout/targeting.go` — mirror it exactly) at manifest time. Send it from the sync worker's manifest call: build, runtime version, OS and version, device class, locale, region setting, channel, hashed attributes from `Plux.setUserContext`.
3. **Reconstruction inputs (SEC-142).** The E8e server records `theme`, `text_scale_percent` and `user_ref` on `ReportInstalledRequest` (`proto/plux/v1/device.proto`); send them in the existing installed-release report (user_ref = the host's pseudonymous user ID from `setUserContext`, never anything else personal).
4. **Mandatory update text:** the key `plux.update.required` exists only in English; add it to the runtime's German, Arabic and Amharic translations (find where the runtime's own strings live: `grep -rn "plux.update.required" lib`).
Tests: `[ABT-002]`/`[ABT-006]` flags and allocations read from a real control document, `[REL-051]` the manifest request carries the targeting fields and only hashes, `[SEC-142]` the report carries theme/text scale/user_ref, the new translations load.
Done when: `cd packages/plux_flutter && flutter test test/control test/experiments test/sync && dart analyze lib test` pass.
