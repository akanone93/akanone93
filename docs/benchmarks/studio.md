<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Studio measurements

## Canvas in software rendering (M-12)

Requirements: STU-045 (canvas frame budget), STU-120 (no-GPU machines), ADR-0013 M-12.

### Method

- Harness: `make studio-bench-canvas` (`studio/packages/canvas/bench/`). Headless Chromium with
  SwiftShader (`--use-angle=swiftshader`, no GPU), viewport 1280 × 800 CSS px at DPR 1, the pinned
  Flutter CanvasKit, tile size 512 px, renderer `auto` (it picks WebGL on SwiftShader by measurement).
- Scene A: 1,400 items at 60 % zoom. Scene B: the large scene at 30 % zoom.
- Scenarios, 30 warm-up frames then 300 measured:
  - **Pan** — 4 × 1 CSS px per animation frame. Before: the worker frame time (record, GL flush, GPU
    sync). After: the interval between consecutive `requestAnimationFrame` callbacks on the page,
    because a pan now moves the base in the compositor and does no GL work per frame. Also reported:
    dropped frames (interval > 25 ms), main-thread work per frame, and base re-renders.
  - **Drag** — one box with its links moved 2 × 1 world units per frame; worker frame time including
    the GPU sync. After the change this is the interaction layer only.
- Three runs per configuration; the table gives the median of the three p95 values and the spread.
- Budget: 16.7 ms p95 (60 Hz).

### Design under test

- The base is rendered once into an overscanned surface (view plus one tile on each side) and moved
  by the host with CSS `translate3d()`/`scale()`; the worker re-renders it when the view comes within
  a quarter margin of its edge (in idle time, once the new tiles are ready), when a pan stops, and
  when a zoom gesture settles.
- Dragged items and their re-routed links are drawn every frame into a second, raster
  OffscreenCanvas above the base, sized to the visible bounds of what moves in 128 px steps; at drag
  end only the touched tiles are re-rendered.

### Results (p95, ms; median of three runs, spread in brackets)

| Scenario | Before | After | Budget met |
|---|---|---|---|
| Scene A pan | 46.11 (34.14–47.80) | 16.67 (16.67–16.67) | yes |
| Scene A drag | 66.23 (51.88–76.44) | 12.29 (10.49–12.72) | yes |
| Scene B pan | 31.97 (30.55–47.20) | 16.67 (16.67–33.33) | yes, one run of three missed |

Main-thread work per pan frame stays below 0.7 ms (p95). The interaction layer draws in about 5 ms
per frame (mean).

### Remaining cost: base re-renders

Each pan of 1,200 px re-renders the base three times. With WebGL on SwiftShader, the tile renders
and the base blit run in the browser's single GPU thread, which also composites the page; each
re-render stalls the compositor for 0.4–1.5 s (max rAF interval per run: Scene A 517–1,483 ms,
Scene B 1,133–1,383 ms; 4–16 dropped frames per 300). That is below the p95 but visible as a hitch.

The same build with the raster renderer forced (`PLUX_CANVAS_RENDERER=raster`, two runs) renders on
the CPU in the worker and keeps the compositor free: pan p95 16.67 ms in both scenes, max rAF
interval 33–50 ms (Scene A) and 16.67 ms (Scene B), 0–3 dropped frames, drag p95 9.13–15.58 ms;
base re-renders take 0.3–0.6 s in the worker and settling takes 6.6–12.2 s instead of 2.8–7.8 s.
Choosing the raster base on software GL is a change of the renderer selection and is left for a
decision.
