<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E8g — the viewer's trust anchor (`SEC-142`, `SEC-051`, `SEC-052`)

`apps/plux_viewer` (merged) verifies a reconstruction with the `targets` keys it fetches from `GetRootKeys` over TLS (`apps/plux_viewer/lib/src/api.dart` `targetKeys`, used in `lib/src/opener.dart`), so a server or anyone in the TLS path could sign a forged reconstruction. Host apps instead pin the installation's **root** keys at build time (`apps/plux_bank/lib/src/config.dart` `rootKeys`) and the runtime walks the signed root chain to the `targets` keys (`packages/plux_flutter/lib/src/sync/metadata_sync.dart`).
Do only:
1. The viewer takes its root keys at build time (`--dart-define=PLUX_VIEWER_ROOT_KEYS=<json>` in the format `PluxConfig` already parses, or a bundled asset — pick the one plux_bank uses) and refuses to open anything without them.
2. Reconstruction mode resolves the `targets` keys through the same verified root chain as a normal sync (reuse `metadata_sync.dart`'s verification; do not write a second one), instead of trusting keys handed to it. Remove `targetKeys` from the viewer.
3. Export `PluxLimit` (or the one bound the viewer needs) from `package:plux_flutter`'s public library so the viewer stops importing `src/`.
4. Document the build-time key in `apps/plux_viewer/README.md`.
Tests `[SEC-142]`: a reconstruction signed by a key outside the pinned chain is refused; a valid chain opens; no root keys → refusal.
Done when: `cd packages/plux_flutter && flutter test test/reconstruction && dart analyze lib test` and `cd apps/plux_viewer && flutter test && dart analyze` pass.
