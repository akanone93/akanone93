<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E8f — the viewer host and the runtime's reconstruction mode (`SEC-142` device half)

Read: `docs/plans/p9.md` lines 566–575 (§5.9 Reconstruction) and the Q14 row of §2.1; the server contract in `backend/internal/reconstruct/doc.go` and `schema/json/reconstruction.schema.json` (the viewer opens `plux-viewer://reconstruction/<id>?server=<url>`, calls `AuditService/GetReconstruction`, which returns the canonical manifest, signatures — verify with `GetRootKeys`, role `targets` — and a bundle URL per bundle); `apps/plux_bank` (an existing host app: copy its structure, not its features); `packages/plux_flutter/lib/src/core/config.dart` (`PluxConfig`) and the sync/verify code it uses (`lib/src/sync`, `lib/src/verify`).

Deliver:
1. **Runtime reconstruction mode** (`PluxConfig.reconstruction` or a dedicated entry point — choose the smallest additive API): loads the reconstruction release's bundles (verify before load exactly as for a normal release, SEC-052), applies the recorded locale, theme, text scale and device class, opens the recorded route with its parameters, shows a non-dismissable banner naming the reconstructed moment, makes no network calls beyond the Plux server, and runs no action with side effects (data source writes, functions with effects, navigation out of the app, host actions — they report a documented reconstruction error; reuse an existing registered code or report that one is needed).
2. **`apps/plux_viewer`**: a minimal Flutter host (Android and iOS) that signs the auditor in (device-code flow of the CLI's device grant, or a pasted token — choose the simplest secure option and document it), handles the `plux-viewer://` link, lists the auditor's reconstructions, and opens one in reconstruction mode; every open is audited server-side already.
3. Tests: `[SEC-142]` reconstruction mode refuses side-effecting actions, verifies before load, shows the banner, applies the recorded presentation; the viewer's link parsing and list screen (widget tests). No emulator runs here (CI only).
Done when: `cd packages/plux_flutter && flutter test <your tests> && dart analyze lib test` and `cd apps/plux_viewer && flutter test && dart analyze` pass.
