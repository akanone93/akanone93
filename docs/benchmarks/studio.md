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

## Bundled arrows while dragging (STU-045, S3.2)

Requirement: STU-045 (all connected bundles re-route and re-select their sides live at 60 fps, for
200 boxes and 1,000 links), with ADR-0013 M-12/M-13 (no GPU).

### Method

- Harness: `make studio-bench-canvas`, scene C (`sceneC` in `bench/scenes.ts`): the 200 boxes of
  scene A as plugins of five screens each, and its 1,000 seeded links as navigate actions between
  screens. They are derived and bundled (`deriveAppBundles`), routed (`ArrowRouter`) and drawn as
  scene items (`ArrowCanvas`) — 988 bundles, 3,486 items — at 60 % zoom, same browser and flags as
  above.
- Pan: as for scenes A and B (rAF to rAF on the page). Drag: box 105 swept 2 × 1 world units per
  animation frame, 30 warm-up and 300 measured frames, timed rAF to rAF; every frame calls
  `ArrowCanvas.moveBox` on the main thread (re-select sides with hysteresis, re-spread anchors,
  re-route with obstacle avoidance, build the patches) and sends the patch. `beginDrag` lifts the
  dragged box's arrows into the interaction layer; the tile re-render this causes is waited out
  before measuring and reported as drag start. `endDrag` re-routes what the drag held back.
- Three runs; the table gives the median and the spread.

### Results (median of 3 runs, spread in brackets, software rendering, SwiftShader)

| Metric | Result | Target | Met |
|---|---|---|---|
| Pan p95 / longest gap | 16.67 (16.67–16.67) / 16.67 (16.67–50.01) ms | 16.7 / 50 ms | yes |
| Re-route per drag frame, p95 / max | 1.27 (0.98–1.93) / 6.96 (6.63–11.89) ms | fits a 16.7 ms frame | yes |
| Main-thread work per drag frame (route + patch), p95 | 5.31 (4.81–5.35) ms | — | — |
| Worker frame while dragging, p95 | 5.73 (5.56–7.02) ms | 16.7 ms | yes |
| Drag, rAF interval p95 | 50.00 (33.33–66.66) ms | 16.7 ms | **no** |
| Drag, longest gap | 133.33 (99.99–149.99) ms | 50 ms | **no** |
| Drag, dropped frames of 300 | 40 (37–74) | — | — |
| Drag start (tiles under the lifted arrows) | 1,003 (986–1,102) ms | — | — |
| Re-route at drop (`endDrag`) | 19.79 (17.50–25.67) ms | — | — |
| First route of all 988 bundles | 143 (82–150) ms | — | — |

Routing fits the frame with room to spare: a drag re-routes only the dragged box's bundles
(23 patches per frame on average), and a move outside a drag only the bundles whose tried
candidates meet the box's old or new place, so the result always equals a full re-route
(`arrows.test.ts`). The worker draws each drag frame in under 6 ms (p95) and does no base render.

The 60 fps cadence while dragging is **not met** in this environment: frames are delivered at a
p95 interval of 50 ms although neither the main thread (5.3 ms) nor the worker (5.7 ms) is busy for
long. The dragged box's 10 bundles in this seeded scene are random links across the whole canvas,
so the interaction layer covers the full viewport; presenting it every frame through SwiftShader
is the remaining suspect, not routing. Scene A's drag figure is a worker frame time, not a rAF
interval, so it does not show this. Follow-up: measure scene A's drag rAF-paced, and limit the
interaction layer to the visible part of the lifted arrows.

Only 47 of the 988 bundles route clear of every box: random links cross a grid of 200 boxes, and
the router detours around at most two boxes (STU-043 makes avoidance a SHOULD); links between
nearby boxes, the common case in an app, route clear (`arrows.test.ts`).

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
(clip to the dirty rectangle) and yielding idle tile work to a pending message — done in the next step,
below.

### Result after damage-rect redraw and preemptible idle work (median of 3 runs, software rendering, SwiftShader, raster backend)

| run | p50 | p95 | max | layoutP95 | frameP95 | flushP95 | budget |
|---|---|---|---|---|---|---|---|
| 1 | 16.7 ms | 17.0 ms | 38.5 ms | 8.5 ms | 0.08 ms | 0 ms | 50 ms p95 |
| 2 | 16.7 ms | 20.0 ms | 52.0 ms | 9.4 ms | 0.07 ms | 0 ms | 50 ms p95 |
| 3 | 16.7 ms | 17.2 ms | 35.3 ms | 7.7 ms | 0.08 ms | 0 ms | 50 ms p95 |
| **median** | **16.7 ms** | **17.2 ms** | 38.5 ms | 8.5 ms | 0.08 ms | 0 ms | 50 ms p95 |

The budget is met (p95 17.2 ms against 50 ms); the time is now the wait for the next animation frame. Three
changes did it:

- **Damage per item** (`damageOf` in `scene.ts`): a box that keeps its fill, stroke and radius and only moves
  edges damages a strip around each moved edge (stroke, radius and one pixel of antialiasing wide), not its
  whole bounds, so the widened body column no longer dirties the screen; other changes damage the old and the
  new bounds.
- **Damage-rect redraw** (`drawDamage` in `renderer.ts`): when the surface already shows every visible tile at
  this placement, a frame redraws only the damaged rectangles, clipped over the pixels the surface keeps, and
  flushes only their union — no tile is blitted. In idle time a dirty tile is refreshed in place: its cached
  image is drawn and only its dirty rectangle re-rendered over it; a tile the surface already shows needs no
  further frame.
- **Preemptible idle work** (`worker.ts`): an idle slice yields when a message has asked for a frame (and
  between tiles), so an edit's frame never waits behind tile rendering; the frame asks for idle time again.

The other canvas targets still hold in these runs: pan p95 16.7 ms in both scenes, longest pan gap at most
50.0 ms, drag p95 10.3–18.1 ms (median 11.8 ms), visible settle at most 58.8 ms.
