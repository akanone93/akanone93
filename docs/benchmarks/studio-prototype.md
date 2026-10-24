<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Studio canvas prototype measurements (P11 S0.8)

Harness: `studio/prototype/canvas/` (`bun run measure.ts`, `bun test`). Raw
output of the run is produced by `measure.ts`; this page records one run.

## Environment

- CPU: Intel(R) Xeon(R) Processor @ 2.10GHz (4 logical CPUs), virtualised.
- Browser: Chromium 153.0.8010.12, headless, software GL (ANGLE over SwiftShader Vulkan), no GPU.
- Toolchain: Go 1.27.3 (`GOOS=js GOARCH=wasm`, `-trimpath -ldflags='-s -w'`), Bun, Playwright 1.63.0.

## CanvasKit

- Source: Flutter 3.47.5 web SDK precache (`canvaskit.js` + `canvaskit.wasm`). The trimmed build was not made (network policy blocks skia.googlesource.com and emsdk). npm `canvaskit-wasm` 0.42.0 is 7,317,345 bytes raw, 2,939,182 gzip -9 for comparison.
- `canvaskit.wasm`: 7,284,602 bytes raw, 2,919,077 gzip.
- `canvaskit.js`: 86,987 bytes raw, 27,582 gzip.
- Worker with OffscreenCanvas in Chromium: works (OffscreenCanvas and worker requestAnimationFrame both present; WebGL renderer is the SwiftShader one above). Firefox and WebKit: not measured here.

## Scene A: 200 boxes, 1,000 Bezier links

Draw calls per frame: 1,600 (no culling). Pan/zoom pass, frame time in the worker:

| GPU sync | p50 | p95 | raf fps |
|---|---|---|---|
| no | 7.2 ms | 434.5 ms | 5.9 |
| yes | 33.2 ms | 448.3 ms | 5.4 |

Main thread for comparison: p50 7.9 ms / p95 415.0 ms (no sync), p50 30.7 ms / p95 443.8 ms (sync). The p95 is dominated by software rasterisation of the Bezier strokes on SwiftShader; a GPU is expected to change this materially and this run cannot say by how much.

## Scene B: 100 phone screens (700 nodes) from the starter plugin pages

- Build 65.3 ms; total paragraph layout 43.6 ms; first frame 31.8 ms; first draw 97.2 ms.
- Pan frame time in the worker:

| Pass | GPU sync | p50 | p95 | draw calls/frame | raf fps |
|---|---|---|---|---|---|
| 0.3x, all 100 screens drawn | no | 3.6 ms | 5.5 ms | 2,300 | 54.7 |
| 0.3x, visible screens only | no | 1.8 ms | 3.1 ms | 794.5 | 57.1 |
| 1.0x, visible screens only | no | 0.5 ms | 0.8 ms | 160.6 | 60.0 |
| 0.3x, all 100 screens drawn | yes | 21.0 ms | 27.2 ms | 2,300 | 42.7 |
| 0.3x, visible screens only | yes | 20.1 ms | 24.3 ms | 794.5 | 44.4 |
| 1.0x, visible screens only | yes | 11.9 ms | 17.1 ms | 160.6 | 54.7 |

## ValidatePage (Go to WebAssembly, in a worker)

- `validator.wasm`: 20,042,647 bytes raw, 4,528,844 gzip (4,533,034 in a later `gzip -9` of the /tmp build).
- 17 project files; instantiate 234 to 814 ms (two runs); load project 372 to 388 ms; 0 diagnostics.
- Cold first call: 38.8 to 46.0 ms (two runs).
- Warm, 50 runs: p50 4.8 ms / p95 15.9 ms (run 1); p50 6.1 ms / p95 18.2 ms (run 2, mean 8.3, max 20.3).

## happy-dom with Base UI

`bun test` in `studio/prototype/canvas`, 12 tests (6 components x 2 package names, `@base-ui-components/react` 1.0.0-rc.0 and `@base-ui/react` 1.9.0): 10 pass, 2 fail.

| Component | Both packages |
|---|---|
| Dialog (open, label, Escape) | pass |
| Popover (dialog role, aria-expanded) | pass |
| Menu (menuitems, close on select) | pass |
| Select (listbox, option selection) | pass |
| Tabs (tablist, tab, tabpanel, switch) | pass |
| Tooltip (hover shows text) | fail: the text appears but the `role="tooltip"` assertion on that element got `null` (role may sit on another element; not investigated) |

## Not measured here

- Trimmed CanvasKit build (network policy).
- Firefox and WebKit.
- A real GPU.
- The reference laptop (M-4).

## Go/no-go inputs

| Requirement | Target | Measured here | Note |
|---|---|---|---|
| STU-120 | 60 fps (16.7 ms frame) at 200 boxes / 1,000 links | worker p95 434.5 ms, p50 7.2 ms (software GL) | no GPU; reference laptop not measured |
| STU-121 | 100 screens | 100 screens drawn: p95 5.5 ms at 0.3x, 0.8 ms at 1.0x visible (no sync); 27.2 ms p95 with GPU sync | first draw 97.2 ms |
| STU-122 | 1.5 MiB initial JS, CanvasKit loaded on demand (not counted) | CanvasKit 7.28 MB raw / 2.92 MB gzip, loaded on demand; validator wasm 4.53 MB gzip; initial app JS not measured | trimmed build not made |
