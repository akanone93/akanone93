<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Changelog

## 0.1.0

The first release: `PluxSecurity`, runtime application self-protection (`SEC-070`, `SEC-071`, ADR-0057). Detects root and jailbreak, hooking frameworks, debuggers, emulators and simulators, repackaging, untrusted installers, screen mirroring and recording, overlays and accessibility services with suspicious capabilities with Plux's own Kotlin and Swift checks; responds per detection with `report`, `warn`, `degrade` or `block`; reports every detection as the `rasp_detection` telemetry event; `PluxSecurityGate` shows the warning and the blocking screen.
