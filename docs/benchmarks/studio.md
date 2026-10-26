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
| Scene A drag (rAF interval; S3.2 before, S3.2c after) | 66.67 | 16.67 | yes |
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
  dragged box's arrows into the interaction layer: the first frame after it is the lift frame, and
  the tile refresh it causes is waited out before measuring and reported as drag start settle.
  `endDrag` re-routes what the drag held back.
- Scene A's drag (box 105 and its links, re-computed by the page) is paced and timed the same
  way, rAF to rAF; the worker's own frame time is reported beside it. Until S3.2b it was a worker
  frame time only, which hid what the browser spends presenting the frame.
- `make studio-bench-canvas` runs the page three times and prints the median of every number.

### Results after damage-only presenting of the interaction layer (S3.2b; median of 3 runs, software rendering, SwiftShader)

| Metric | Before (S3.2) | After | Target | Met |
|---|---|---|---|---|
| Scene C pan, p95 / longest gap | 16.67 / 16.67 ms | 16.67 / 16.67 ms | 16.7 / 50 ms | yes |
| Scene C drag, rAF interval p95 | 50.00 ms | 16.67 ms | 16.7 ms | yes |
| Scene C drag, longest gap | 183.32 ms | 16.67 ms | 50 ms | yes |
| Scene C drag, dropped frames of 300 | 50 | 0 | — | — |
| Scene A drag, rAF interval p95 | 66.67 ms | 16.67 ms | 16.7 ms | yes |
| Scene A drag, longest gap | 166.66 ms | 33.34 ms | 50 ms | yes |
| Scene A drag, dropped frames of 300 | 60 | 8 | — | — |
| Worker frame while dragging, p95 (A / C) | 12.56 / 5.39 ms | 8.56 / 5.16 ms | 16.7 ms | yes |
| Layer pixels presented per drag frame, mean (A / C) | whole layer: 1,769,472 / 1,179,648 | 346,962 / 222,073 | — | — |
| Re-route per drag frame, p95 | 0.95 ms | 0.60 ms | fits a 16.7 ms frame | yes |
| Main-thread work per drag frame (route + patch), p95 | 3.68 ms | 4.79 ms | — | — |
| Lift frame (first frame after `beginDrag`) | 134.90 ms | 78.87 ms | one frame, 16.7 ms | **no** |
| Drag start settle (7 tiles refreshed under the lifted arrows) | 1,029 ms | 595 ms | 300 ms (M-13) | **no** |
| Re-route at drop (`endDrag`) | 26.65 ms | 15.31 ms | — | — |
| First route of all 988 bundles | 67.42 ms | 105.16 ms | — | — |

Routing fits the frame with room to spare: a drag re-routes only the dragged box's bundles
(23 patches per frame on average), and a move outside a drag only the bundles whose tried
candidates meet the box's old or new place, so the result always equals a full re-route
(`arrows.test.ts`).

The S3.2 run left drag at 50 ms p95 although neither thread was busy. One instrumented run
proved the cause: skipping only the interaction layer's `flush` (its `putImageData` to the
OffscreenCanvas) brought scene C's drag from p95 50 / longest 100 ms to 16.67 / 33.34 ms, while the
worker's layer draw fell by under 1 ms. Flushing a 256 × 256 dirty rectangle instead gave the same
16.67 ms p95. The cost was the browser uploading and compositing the whole layer through
SwiftShader on every frame, not drawing or routing.

The fix keeps the layer and its draw whole but presents only its damage: the scene records where
live items changed (old and new bounds; a link as eight pieces along its curve and along the
straight line that level of detail draws, so a long diagonal link damages a band and not its
bounding box), the renderer merges these into at most 48 regions and copies only those to the
canvas. The layer is drawn whole and unclipped, so its pixels are identical to a full draw
(`renderer.test.ts` compares each frame and what the canvas shows, pixel for pixel). Lifting and
dropping now invalidate the tiles along the lifted links, not under their bounding boxes, and
every add, remove, lift and drop damages the antialiasing fringe around an item too. The new test
found the latter: a dropped box left its antialiased top edge in the tile.

Still missed: the lift (scene A's longest gap of 33.34 ms, 8 frames of 300 dropped, is within the
50 ms target). The lift frame redraws the base under every lifted link (78.87 ms), and
the seven tiles refreshed in idle time take about 85 ms each, 595 ms in all. Meeting one frame and
300 ms needs the lift not to touch the tiles at all, for example by masking the lifted items out
of the base instead of re-rendering it; that is a follow-up.

Only 47 of the 988 bundles route clear of every box: random links cross a grid of 200 boxes, and
the router detours around at most two boxes (STU-043 makes avoidance a SHOULD); links between
nearby boxes, the common case in an app, route clear (`arrows.test.ts`).


### Results after lifting and dropping without re-rendering tiles (S3.2c; median of 3 runs, software rendering, SwiftShader)

A lift no longer touches the tiles. The lifted items stay in the cached tiles and are masked out of
the base by patches: their pieces are redrawn without them, behind one non-antialiased clip of all
the pieces of a chunk, in preemptible idle time after the lift's frame is shown, chunked by
tile-sized cell and flushed once per idle slice. Every later blit of the tiles (a whole base draw,
a realignment, a progressive job) queues the masks again. A drop draws the dropped items over the
base in its own frame, then patches their pieces in z-order and refreshes the tiles they left and
joined in idle time. `renderer.test.ts` checks that the lift and the drop render no tile in their
frame, and that the base after the lift's patches, after a pan mid-drag and after the drop settles
equals a full re-render (same tolerance as before). Until the patches are drawn, a lifted item's
old pixels can show under the layer for a few frames.

| Metric | Before (S3.2b) | After | Target | Met |
|---|---|---|---|---|
| Lift frame, scene C (worker, first frame after `beginDrag`) | 78.87 ms | 5.20 ms | rAF gap ≤ 50 ms | yes |
| Drag start settle, scene C | 595 ms | 151.63 ms | 300 ms (M-13) | yes |
| Lift frame, scene A (new measurement) | — | 16.24 ms | rAF gap ≤ 50 ms | yes |
| Scene C drag, rAF p95 / longest gap | 16.67 / 16.67 ms | 16.67 / 16.67 ms | 16.7 / 50 ms | yes |
| Scene A drag, rAF p95 / longest gap | 16.67 / 33.34 ms | 16.67 / 33.33 ms (S3.2d; S3.2c: 66.66) | 16.7 / 50 ms | yes |
| Worker frame while dragging, p95 (A / C) | 8.56 / 5.16 ms | 13.97 / 8.02 ms | 16.7 ms | yes |
| Drop frame (worker, first frame after `endInteraction`; A / C) | — | 6.88 / 4.58 ms | — | — |
| Drop settle, scene A (new measurement) | — | 266.70 ms (S3.2d; S3.2c: 511.42) | 300 ms (M-13) | yes |
| Drop settle, scene C (measurement fixed in S3.2d) | — | 244.53 ms | 300 ms (M-13) | yes |
| Drop frame, scene C (worker, the real drop frame; S3.2d) | — | 116.74 ms | — | — |

S3.2d closed the two misses of S3.2c (median of 3 runs, 2026-10-26, idle machine: load average 1.30
at the start, 1.82 at the end, on four cores):

- **Lift masks paced per frame.** While a drag runs, patches may flush at most one tile's device
  pixels (512 × 512) per animation frame — the worker waits for the next frame before resuming them
  — and each tile-sized cell's chunks are flushed on their own instead of as one union of distant
  cells. Every flush of the base is an upload on the browser's single software-GL thread, which also
  composites the page, so this bounds what a drag frame can wait for. Scene A's longest drag gap:
  33.33 ms (2 frames of 300 dropped), was 66.66 ms.
- **Region refresh after a drop.** A tile keeps its dirty pieces apart (at most 64, clipped to the
  tile; more merge into the piece they grow least), and idle time redraws only those pieces, 32 per
  pass behind one non-antialiased clip, preemptible between passes. Pieces the settled base shows
  (a CPU raster base at the tiles' scale, nothing left to patch or draw) are copied from it, pixel
  for pixel, instead of redrawn. Outside an interaction, patches are drawn in chunks of 32 and
  flushed once per idle slice. Drop settle: scene A 266.70 ms (10 tile refreshes, 57 ms), scene C
  244.53 ms (13 refreshes, 101 ms).
- **Scene C's drop settle measured.** The bench now waits for the drop's own frame (the first
  without live items) before it counts settle frames; drag frames still queued had ended the wait
  at 0.70 ms. The same fix exposes scene C's real drop frame: 116.74 ms in the worker, mostly the
  rerouted arrows of `endDrag` drawn as regions. It has no target; it is the next cost to cut.

`renderer.test.ts` checks that the lift's patches are paced, that the drop settles slice by slice
with tile pieces refreshed (some copied from the base), and that the base after the drop and a
whole draw from the refreshed tiles equal a full re-render (same tolerance as before). Other
numbers of the same runs: lift frame 9.81 ms (A) and 5.69 ms (C), drag start settle 138.04 ms (C),
scene C drag p95 / longest gap 16.67 / 16.67 ms, worker frame while dragging p95 10.28 / 8.59 ms,
pan p95 and longest gap 16.67 ms in scenes A, B and C. Earlier S3.2c runs shared the machine with
other builds (load average 2–3); runs beside heavier load (6–13) are not comparable.

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

## Full-screen overlay open (STU-123)

Requirement: STU-123 — opening a full-screen overlay takes at most 150 ms after its code is loaded.

- Harness: `make studio-bench-overlay` (`studio/apps/web/bench/`). Headless Chromium with SwiftShader (no GPU), as for
  the canvas. The page holds 400 paragraphs standing in for the canvas; an overlay is a full-viewport container in which
  Monaco's JSON editor is created on a document of 2 726 lines, with the same options as the JSON editor. The time is
  measured from the call that opens the overlay to the second animation frame after the editor exists, the first frame
  that shows the text. Monaco's code is loaded and warmed by two opens before the 21 measured runs, because the
  requirement counts from the moment the code is loaded. The page runs three times; the median of each number is printed
  and the run fails when p95 exceeds 150 ms.
- Not measured here: loading Monaco's chunk itself (it is lazy; the initial JavaScript is held by `make studio-size`,
  STU-122), and the canvas worker's pause, which is a message and no work.

### Result (median of 3 runs, software rendering, SwiftShader; 21 opens per run after 2 warm-up opens)

| p50 | p95 | max | budget |
|---|---|---|---|
| 33.0 ms | 54.2 ms | 143.3 ms | 150 ms |

The budget is met: p95 is 54.2 ms and even the slowest open of the median run, 143.3 ms, is inside 150 ms. An earlier
run on a machine that was building at the same time gave p95 117.4 ms and a maximum of 191.6 ms, so the number is
sensitive to load; the budget is judged on p95 on an otherwise idle machine.
