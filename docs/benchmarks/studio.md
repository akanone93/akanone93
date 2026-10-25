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

Diagnosis (branches `p11/s13-gap`, `p11/s13-tiles`, not merged): splitting the work did not help.
With every worker job at most 2.5 ms of CPU, the GPU process still stalled 400–870 ms after each
recentre on work submitted by the worker's WebGL context — invisible to the worker's own timings —
and the gap stayed at 300–750 ms. The forced raster renderer had no stall.

Result: on software GL the canvas now uses CanvasKit's CPU raster backend (ADR-0013), chosen from
the GL probe (renderer string SwiftShader, llvmpipe, … or a context refused for
`failIfMajorPerformanceCaveat`); the start-up measurement is gone, as it could not see the GPU
process. Measured the same day, median of three runs:

| Renderer (Scene A / Scene B pan) | p95 | Longest gap | Settle visible | Base job max |
|---|---|---|---|---|
| WebGL, forced | 16.67 / 16.67 | 383 / 383 | 130 / 108 | 408 / 427 |
| Raster via `MakeSWCanvasSurface` | 16.67 / 16.67 | 16.67 / 16.67 | 223 / 277 | 221 / 284 |
| Raster, premultiplied, dirty-rect present (this change; four runs) | 16.67 / 16.67 | 16.67 / 16.67 | 38 / 41 | 28 / 28 |

With this change every target is met in the median: pan p95 16.67 ms, longest gap 16.67 ms (one
run of four had 50 ms in Scene A), drag p95 4.1–6.7 ms (one run had a single 132 ms drag frame),
visible settle about 40 ms and full settle 60–110 ms. These four runs drew the drag layer through
the premultiplied surface; the final build returns it to `MakeSWCanvasSurface` (its anti-aliased
edges are transparent) and was measured again, median of three: pan p95 16.67 / 16.67 ms, longest
gap 16.67 / 16.67 ms (single runs up to 66.67 ms in Scene A and 49.99 ms in Scene B), visible settle
40 / 41 ms, drag p95 7.2 ms (max 46.5 ms).

`MakeSWCanvasSurface` renders unpremultiplied RGBA, which takes Skia off its fast blitters (a
realignment of 20 tile blits took 220–330 ms) and copies the whole 2,304 × 1,824 base to the
canvas on every flush. The base now renders into a premultiplied raster surface — it is opaque, so
its bytes are valid `putImageData` input — and a tile job copies only the tile's rectangle, into a
CPU-backed 2D context (`willReadFrequently`; an accelerated one put the copies back on SwiftShader
and brought back single 670–1,130 ms gaps in Scene B in four of seven runs). The transparent drag
layer keeps `MakeSWCanvasSurface`.

## Property change to canvas frame (STU-121, NFR-032)

Requirement: a property change reaches the canvas frame in 50 ms at p95.

### Method

- Harness: the `propertyToCanvas` scenario of `make studio-bench-canvas`
  (`studio/packages/canvas/bench/property.ts`, run by `page.ts`); same software-rendering setup as above.
- Scene: eight screens of forty text rows each. Each iteration edits one text property (`data`) of one node,
  hands all screens to `CanvasHost.layoutScreens`, then forces and awaits the next frame. Since S2.6 (live
  canvas) the host sends only the changed screen (`screensUpdate`), the worker lays out only that screen and
  answers with `replace` ops for the items that differ, so only the tiles under them redraw. Time = hand-over to that frame; 30 warm-up and 300 measured iterations.
- Not included: the main-thread Immer recipe of `edit`, which is microseconds for one field.
- `layoutP95` is the part up to the worker's layout answer; the rest is the frame. `frameP95` is the worker's
  own drawing time of that frame (`FrameStats.ms`), `flushP95` its flush.

### Result (one run, software rendering, SwiftShader)

| p50 | p95 | max | layoutP95 | budget |
|---|---|---|---|---|
| 72.4 ms | 101.0 ms | 165.3 ms | 54.0 ms | 50 ms p95 |

Before the live canvas the worker laid out all 320 paragraphs and redrew the whole scene for a one-field change.

### Result after per-screen relayout (median of 3 runs, software rendering, SwiftShader, raster backend)

| run | p50 | p95 | max | layoutP95 | frameP95 | budget |
|---|---|---|---|---|---|---|
| 1 | 50.7 ms | 71.7 ms | 123.0 ms | 36.8 ms | — | 50 ms p95 |
| 2 | 50.1 ms | 62.7 ms | 100.5 ms | 32.8 ms | 29.7 ms | 50 ms p95 |
| 3 | 49.9 ms | 66.6 ms | 132.2 ms | 33.6 ms | 33.8 ms | 50 ms p95 |
| **median** | **50.1 ms** | **66.6 ms** | 123.0 ms | 33.6 ms | | 50 ms p95 |

The budget is **still not met** (p95 66.6 ms against 50 ms). Laying out the edited screen alone takes about
3 ms in the worker (measured in isolation; 8 screens took about 97 ms), and an edit changes one or two items
(the text and, as the column shrink-wraps its widest row, the body box). What remains is the drawing: the
body box spans the whole screen, so every tile under the screen renders again — about 30 ms p95 per frame in
SwiftShader raster, flush about 5 ms — and the next update waits behind the idle tile work of the previous
frame (`layoutP95` about 33 ms against about 3 ms of layout). The next step is partial tile re-rendering
(clip to the dirty rectangle) and yielding idle tile work to a pending message; tracked in the work log. The
requirement is therefore not marked `DONE`.
