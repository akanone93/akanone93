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

### Progressive, preemptible base re-render (S1.3)

Design: a recentred base is re-rendered as tile jobs (`progressive.ts`) under an 8 ms budget per
animation frame: the realignment first — the cached tiles blitted at the new placement, older-scale
tiles scaled underneath as the interim, no read-back of the base — then the tiles the view shows,
centre-out, then the margin ahead of the pan, then the rest. A tile renders in four strips, one job
each. A new viewport replans the queue; cached tiles whose content and scale did not change are
reused, so a pan renders only newly exposed tiles.

Targets (software GL, same harness): pan and drag p95 ≤ 16.7 ms, longest rAF gap ≤ 50 ms, **settle
≤ 300 ms** — from gesture end until every tile intersecting the visible viewport is at full
resolution (`settleVisibleMs`); the overscan's completion is reported separately (`settleFullMs`).

Results, one run (2026-10-25), WebGL on SwiftShader:

| Scenario | p95 | Longest gap | Dropped | Settle visible | Settle full | Met |
|---|---|---|---|---|---|---|
| Scene A pan | 16.67 | 583.31 | 8 | 133.06 | 247.12 | p95, settle; gap no |
| Scene A drag | 8.07 | 17.77 | — | — | — | yes |
| Scene B pan | 16.67 | 333.33 | 3 | 120.38 | 149.51 | p95, settle; gap no |

The longest gap is the realignment job alone (`worstBase`: one job, 25 tile blits, 607 ms in
Scene A and 379 ms in Scene B): repainting the whole overscanned base (2,304 × 1,824 px) in one GL
submission occupies SwiftShader's GPU thread, which also composites the page. Splitting it is not
possible on one canvas, as the host moves the whole base with one transform; meeting 50 ms needs a
second base canvas, filled by budgeted blit jobs and swapped in when complete (open).
