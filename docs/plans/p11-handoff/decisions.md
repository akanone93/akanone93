<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Maintainer decisions taken in session 1

Earlier P11 decisions (M-1–M-13, O-1–O-20, L-1–L-15) are in [p11.md](../p11.md) §2.

| Date | Decision | Where recorded |
|---|---|---|
| 2026-10-25 | Hit-testing and the overlay run on the main thread | ADR-0013 revision |
| 2026-10-25 | One Trusted Types policy, `plux-worker`, for Studio's own workers | studio-development.md §11.2 |
| 2026-10-25 | F5: move, nudge and resize follow the parent | p11-studio-functional-decisions.md |
| 2026-10-25 | Dedicated upload error codes; fix the sign-in cookie race and the per-process OIDC state secret | code, PLX-6032..6037 |
| 2026-10-25 | Canvas settle ≤ 300 ms with no GPU (M-13) | p11.md, ADR-0013 |
| 2026-10-26 | Limits: `studio.changeSetDocuments` 500 (max 5000); `metadata.*` description 2000, 20 tags ≤ 40 chars, icon id ≤ 200 | schema/limits.json, spec §30.4 |
| 2026-10-26 | First-party plugin packages (plux_charts, plux_lottie, plux_maps, plux_markdown, plux_rive, plux_player, plux_media) as `plux_flutter` dev_dependencies for the Layer 2 golden writer; plux_scanner excluded (build-hook cycle), scanners checked as a box only | packages/plux_flutter/pubspec.yaml, screen.test.ts |
| 2026-10-26 | Design gate ignores `undersized-ui-text`, `dark-glow`, `pulsing-dot` for the approved Porcelain tokens — **revisit later** | studio/.impeccable/config.json, work log Open decisions |
| 2026-10-26 | Design-time view of If/Match/ForEach/Slot/Responsive/DataScope | docs/reference/document-model.md §4.1 |
| 2026-10-26 | CSP `style-src-attr 'unsafe-inline'` for Monaco's style attributes; style elements stay nonce-only | ADR-0014 revision 2026-10-26, studio-development.md §11.2, bff.test.ts |
| 2026-10-27 | Rust language server and test runner gated by `function.deploy` (no new permission) | S5.4 code |
| 2026-10-27 | Limit `lsp.messageBytes` 1 MiB (max 8 MiB) | schema/limits.json |
| 2026-10-27 | Model tiers: Haiku and Sonnet **low** by default outside the five Opus parts | p11.md M-10, sub-agents.md, [agent-rules.md](agent-rules.md) |
| 2026-10-27 | LSP client: Studio's own typed JSON-RPC client, not `monaco-languageclient` (revisit if needed) | p11-studio-decisions.md A5.3, ADR-0099 |
| 2026-10-27 | S5.4 security defaults confirmed (build scripts, proc macros, check-on-save off; `executeCommand`, `didChangeConfiguration`, `rust-analyzer/*` refused; editor off by default; `builder.address` needs internal TLS; `RunTests` runs stored cases) | S5.4 code |
| 2026-10-27 | Custom device frame (D40): `studio.customFrameMaxPixels` 4096 (max 8192) per side, density 1–4 | schema/limits.json, spec §30.4 |
| 2026-10-26 | Sign-in page design is not changed now | — |
