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

## No-GPU techniques (M-12)

Harness: `studio/prototype/canvas/nogpu.ts`, `nogpu-worker.ts`, `nogpu-measure.ts` (`bun run nogpu-measure.ts`). Same machine and Chromium as above (no GPU, SwiftShader), worker with an OffscreenCanvas, 1280x800, 30 warm-up and 300 measured frames per cell. Frame time is CPU time from first draw call to the end of `flush()`; the WebGL surface adds a 1x1 `readPixels` so the GL work is included. "SW" is `MakeSWCanvasSurface(offscreenCanvas)` (CanvasKit CPU raster); `MakeWebGLCanvasSurface` is "GL". Cells are p50 / p95 in ms; one run per cell, and the same configuration varied by up to 1.7x between two full runs (GL baseline pan p95 89.7 then 44.9), so differences under 2x are noise. Table is the second, final run.

Techniques as implemented: **batch** = one SkPath for all links of a style (static set), one path for all box fills and one for all box edges; titles stay one paragraph call each. **tile** = 512x512 CPU raster tiles of the static layer at the current zoom, `makeImageSnapshot`, drawn with `drawImageRectOptions` (nearest), built on first visibility inside the measured frames; tiles are dropped when zoom drifts >25 % (the scenarios pan at a fixed zoom, so this path is implemented but not exercised). **lod** = while interacting: straight links and no titles below 50 % zoom, half resolution (low-res surface blitted up, or half-resolution tiles); full resolution on idle. Scene A has no blur shadows, so that part of LOD is moot. **Drag** (box 105, 9 to 10 links re-routed): without tiles the full scene is redrawn (no dirty regions); with tiles the static layer excludes the dragged box and its links, and only those are redrawn on top each frame. Tile rebuild at drag start is excluded from the frames and shown separately.

| Variant | A pan 0.6x | A pan 0.4x | A drag 0.6x | A draw calls | B pan 0.3x | B draw calls | Idle full redraw (A / B) |
|---|---|---|---|---|---|---|---|
| GL baseline | 30.2 / 44.9 | 32.1 / 53.3 | 33.7 / 55.2 | 1600 | 23.4 / 35.6 | 795 | n/a |
| GL batch | 42.7 / 64.6 | 57.7 / 119.8 | 72.9 / 132.0 | 203 | n/a | n/a | n/a |
| GL batch+tile+lod | 13.5 / 23.3 | 21.2 / 48.8 | 14.2 / 21.3 (rebuild 50) | 8 to 10 | 16.8 / 32.0 (tile+lod) | 14 | 46.6 / 64.7 and 36.7 / 59.3 |
| SW baseline | 25.8 / 44.5 | 27.1 / 41.6 | 33.5 / 50.3 | 1600 | 15.5 / 26.1 | 795 | n/a |
| SW batch | 26.3 / 45.1 | 24.6 / 47.0 | 29.6 / 43.5 | 203 | n/a | n/a | n/a |
| SW tile | 80.9 / 119.9 | 75.8 / 105.1 | 78.9 / 106.2 (rebuild 123) | 9 to 16 | 80.3 / 108.4 | 16 | 27.6 / 42.6 and 18.9 / 31.1 |
| SW lod | 100.2 / 138.6 | 94.6 / 122.4 | 97.5 / 125.8 | 1401 to 1601 | 93.0 / 124.9 | 554 | 32.4 / 43.5 and 20.1 / 27.9 |
| SW batch+tile+lod | 80.9 / 106.2 | 80.5 / 105.1 | 87.2 / 128.9 (rebuild 100) | 8 to 10 | 87.8 / 109.0 (tile+lod) | 14 | 26.8 / 47.4 and 22.0 / 27.1 |

The earlier S0.8 figure for scene A (p50 7.2 / p95 434 ms) came from a harness that did not synchronise the GL work in the unsynchronised pass; here every GL cell is synchronised, so the GL baseline is not comparable to it.

- Renderer under software GL: SW raster and GL are within noise on the untouched scenes (A 25.8 vs 30.2 p50, B 15.5 vs 23.4), so no reason to pick GL when it is software; but every `drawImage*` call on the SW surface cost about 5 ms (A tile variants 76 to 87 ms p50 with 9 to 16 image draws), so the tile cache and half-resolution blit made SW slower, not faster.
- Reaching 16.7 ms p95: only GL batch+tile+lod on scene A (pan 0.6x p95 23.3, drag 21.3, p50 13.5 to 14.2) came close and none met it at p95; batching alone helped neither renderer (draw calls 1600 to 203, no gain on SW, worse on GL).
- Open: why image blits are slow on the CanvasKit SW surface (try `MakeRasterDirectSurface`, `putImageData` of a pixel buffer, or `transferToImageBitmap`); tiles built off the frame (idle or a second worker); a repeat with several runs per cell for confidence intervals; zoom-change invalidation and dirty rectangles are not exercised; a real low-end CPU.
