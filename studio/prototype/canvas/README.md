<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: AGPL-3.0-only
-->

# Plux Canvas prototype (P11 step S0.8)

A measurement harness, not product code. It answers whether the Plux Canvas
design holds: CanvasKit in a Web Worker on an `OffscreenCanvas`, two scenes
(STU-120, STU-121), the Go page validator compiled to WebAssembly, and Base UI
under happy-dom. Results: [docs/benchmarks/studio-prototype.md](../../../docs/benchmarks/studio-prototype.md).

This is a standalone Bun package. It is not part of the Studio workspace and
has its own `bun.lock`.

## Rerun

```sh
cd studio/prototype/canvas && bun install && bun run measure.ts && bun test
```

`bun run measure.ts` builds the validator (`GOOS=js GOARCH=wasm`, needs the Go
toolchain), starts a static server on `127.0.0.1`, drives headless Chromium
with Playwright and prints the numbers as JSON on stdout (also written to
`dist/results.json`). A full run takes about five minutes on software GL.

Options: `--canvaskit=flutter|flutter-chromium|npm|npm-full` picks the
CanvasKit build (default: Flutter's, from `flutter precache --web`, falling
back to npm `canvaskit-wasm`); `--only=validator` skips the canvas scenes.

## Needs

- Bun, Go (the `backend/go.mod` toolchain) and a Playwright Chromium.
- A TTF font for the paragraphs: the Flutter `material_fonts/Roboto-Regular.ttf`
  is used when found, otherwise set `PLUX_FONT`.

`dist/` (including `validator.wasm`) and `node_modules/` are not committed.
`bun test` here runs the happy-dom suite only when started from this directory.
