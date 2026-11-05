<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0013. Plux Canvas: CanvasKit, a port of Flutter's layout, and one canvas worker

- **Status:** **Accepted** (maintainer, 2026-10-24; P11 plan M-9, O-7)
- **Date:** 2026-10-24
- **Requirements:** `STU-003`, `STU-005`, `STU-006`, `STU-120`, `STU-121`

## Context and problem

Studio shows the app being built on an infinite canvas (`STU-003`). What the canvas draws must
match what the device draws (`STU-005`): the same positions and sizes for every widget, in every
device frame, locale and text scale. Text is the hard part. On the device Flutter lays out text
with Skia's SkParagraph, also under Impeller. A renderer that measures text with the browser
cannot reproduce that layout, and Flutter itself removed its browser-text renderer in 3.29.
The question: how does Studio draw and lay out widgets so that the result is provably the
device's, at 60 frames per second on the reference laptop (`STU-120`)?

The preparation record is [p11-studio-decisions.md](../plans/p11-studio-decisions.md) (A4.3,
A4.4, A5.1, A5.4, A5.6); the phase plan is [p11.md](../plans/p11.md) (L-4, O-7, O-10).

## Decision drivers

- Fidelity of text layout and widget geometry to the device (`STU-005`, `STU-006`), above pixel fidelity.
- Frame rate and edit latency that React work can never disturb (`STU-120`, `STU-121`).
- A provable claim: a conformance suite against Flutter itself, not a visual impression.
- Size: the canvas loads on demand, outside the initial JavaScript budget (`STU-122`).
- Licensing that stays clean inside the AGPL `studio/` tree (ADR-0022).

## Considered options

1. CanvasKit from our own trimmed build, a TypeScript port of Flutter's layout code, one canvas worker.
2. PixiJS or raw WebGL2 with a CSS-flexbox layout engine (Yoga, Taffy).
3. Canvas2D libraries (Konva, Fabric), SVG or DOM rendering.
4. Flutter Web, Vello or ThorVG.

## Decision

Chosen option: **1**. The numbers and the rejected alternatives are in decisions A4.3 and A4.4.

### Renderer

- **CanvasKit, built by us** from the Skia revision that Flutter 3.47.5 pins
  (`8df24be66531469e576a806749a0202ae26b8d08`), trimmed (no Skottie, PDF or other extras). The
  toolchain (emsdk) is a build tool only, as `plux-svgc` already builds Skia path ops (ADR-0027).
  The build is reproducible (`CI-006`) and its output is cached immutable.
- **Text is shaped with SkParagraph** at that revision. This is the deciding reason: CanvasKit
  runs the device's text code in the browser. `STU-006` is reworded to say so.
  A text the layout measured is drawn with the families it was measured in and laid out again at
  its measured width rounded up, as Flutter's text painter once did: at the exact floating-point
  width SkParagraph can find the line a hair too long and break it, and the lines below overlap
  (amended 2026-11-04).
- **Screens are painted, not only placed** (amended 2026-11-04, maintainer). The layout engine stays layout-only;
  a paint module beside it (`studio/packages/layout/src/paint/`) says what each widget draws from its props, as
  Flutter's painters do — fills, gradients, borders, per-corner radii, circles, Material elevation shadows (Flutter's
  `kElevationToShadow`), and the clips and opacity its ancestors apply — in paint order. Colours come from the theme
  as the runtime reads it: token paths, dark values, brands and aliases, and the Material 3 scheme from
  `ColorScheme.fromSeed` with the tonal spot variant, ported from material_color_utilities 0.13.0 (the version the
  pinned Flutter uses) and tested role by role against vectors from that library. Layout-only widgets draw nothing;
  widgets not painted yet keep a neutral box. Texts are measured and drawn in one resolved style — Material 3's type
  scale (Flutter's `englishLike2021` metrics), the theme's `typography.<role>` tokens over it, then the widget's
  `style` — with even leading, so a line breaks on the canvas where the layout broke it. A painted screen is laid out
  as the device lays it out: edge to edge, with no design-time page padding or gaps; `Column`, `Row`, `Padding`,
  `SizedBox`, `Center`, `Align`, `Expanded`, `Flexible`, `Container`, `ListView` and `SingleChildScrollView` are
  built by the same scene code the conformance suite verifies, with token values resolved and values only the device
  knows left at their defaults; a `Scaffold` body is as high as the device below the app bar unless it scrolls. An
  `Icon` is its glyph in the server's pinned icon fonts — Material Symbols Outlined at its default instance, in two
  parts, and the Cupertino icons, derived by `studio/scripts/gen-icons.ts` — at its size and colour, else its
  component's (Flutter's icon theme), mirrored in right-to-left text when Flutter mirrors it; fill, weight and grade
  variations are the device's. An `Image` or a `CircleAvatar`'s image of an app asset is drawn from the asset's
  raster form, fitted as Flutter's `paintImage` fits it (every `BoxFit`, alignment) and clipped to its shape; the host
  fetches it and hands it to the worker by key, and it shows a placeholder until it arrives. A web image is the
  device's to load: the canvas draws its placeholder. Material 3's controls — buttons, icon buttons, floating action
  buttons, chips, checkboxes, radios, switches, sliders, progress indicators and badges — are laid out by the scene
  code and painted with the defaults of Flutter's `_*DefaultsM3`; a text inside a component is laid out in the
  component's text role merged with its own style, and drawn in the style it was measured in. A control with no
  action is drawn disabled, as Flutter disables a button whose `onPressed` is null. Text fields are laid out by the
  scene's input decorator from their `decoration`, and painted with `_InputDecoratorDefaultsM3`: underline, filled or
  outlined container, resting or floating label, value or hint, prefix and suffix, helper or error, icons. On a painted
  screen a composite reports the parts it makes up itself (`<id>#input`, `#hint` …) so they are drawn where they were
  laid out; the conformance scenes do not. A painted scaffold lays out and paints its own `AppBar`; navigation bars and
  rails, tab bars, drawers and bottom app bars are painted with their surfaces and their selected destination or tab
  (indicator, colours); list tiles are built by the scene and have no surface of their own. Cupertino widgets are
  built by the scene (their slots in the scene's slot nodes) and painted with iOS's colours from Flutter's
  `CupertinoColors`, light or dark, and the runtime's Cupertino theme; under the **Cupertino** preview a Material
  control marked `adaptive` is painted as its Cupertino counterpart in the box Material lays out. On a painted screen
  the texts a Layer 2 component lays out itself are reported as its parts (`<id>#p<n>`) and drawn in its colours, over
  the surfaces `plux_components` gives it (gradient account and payment cards, Material cards, banners, skeletons,
  underlined inputs); a screen the layout cannot lay out keeps its frame. Charts are drawn from their data (bars,
  lines as segments, pie wedges and progress as arcs); maps, cameras, video, audio, QR codes, animations and native
  slots are previews of what only the device shows. Painting is reviewed by eye against the device, not by pixels.
- Painting follows Skia's model; the device paints with Impeller, so pixels differ slightly.
  Conformance therefore compares positions and sizes, never pixels.
- The canvas module loads on demand, is prefetched while the apps grid is open, is compiled with
  `compileStreaming`, and is cached immutable. Native handles are freed through one ownership
  wrapper with a leak check in tests.

### Layout port

- **`studio/packages/layout`** is a TypeScript port of Flutter's layout code: box constraints
  and sizes, intrinsic dimensions, baselines, the sliver protocol and each render object. CSS
  engines (Yoga, Taffy) implement different rules (flex-shrink, unbounded main axis, tight and
  loose fit) and would fail `STU-005` on their differences.
- The package has no DOM and no React. Each ported file records the **Flutter source revision**
  it came from, so a Flutter upgrade is a diff to port. Relayout boundaries come with the port
  (`STU-121`).
- Ported code keeps Flutter's BSD-3-Clause notice, annotated in `REUSE.toml` inside AGPL `studio/`.
- **Widget scope (L-4):** every descriptor in `schema/widgets/` (149 Layer 1 and 58 Layer 2 at the
  branch point) plus any added before S4 closes.

### Conformance suite and tolerance

- **`test/layout-conformance/`**: headless `flutter_test` writes the global rect of every node
  as golden JSON, per widget × device frame × locale × text scale. `bun test` lays out the same
  scenes with headless CanvasKit and compares.
- **Tolerance:** positions and sizes, not pixels. A box edge may differ by at most **0.5 logical
  pixels**; a size derived from text (a paragraph's width, height or line count's extent) by at
  most **1 logical pixel**. Line counts and break positions must match exactly. The numbers are
  revisited once S1.2 measures real differences, and any change to them is a decision for the
  maintainer (AGENTS §7).
- A Flutter upgrade regenerates the goldens; the diffs are ported, and `layout` keeps a 90 %
  coverage floor (O-10).

### Threading

- **One dedicated canvas worker** owns layout, CanvasKit and the scene graph, and
  draws through `transferControlToOffscreen()`, so no frame is copied. React and the document
  stay on the main thread; edits reach the worker as one JSON Patch stream (A5.1).
- **Hit-testing and the selection overlay run on the main thread** (Revision 2026-10-25): the
  host keeps a grid index of item bounds, fed from the same patch stream, so a hover or click
  answers without a worker round trip; the selection outline and handles are drawn on a small
  main-thread canvas above the base, so selecting never re-renders the base. The cost is a
  second copy of the item bounds in memory. The canvases of Studio draw that feedback in the page's DOM over the
  canvas instead (amended 2026-11-05): the plugin canvas's overlay — selection with its name, size and handles,
  hover, marquee, snap guides, distances, insertion indicator and drop target — follows every pan and zoom from the
  host's view and the hit index's bounds, so it too never re-renders the base. A screen that only moved (its tree and
  title the same) is translated in the worker with `move` ops (`ScreenMove`), not laid out again, so dragging a screen
  costs no layout; moving an item moves the world points it paints by (its ancestors' clips, its gradient).
- One worker, not two: layout needs SkParagraph, and splitting would load CanvasKit and fonts twice.
- The module talks only through its message interface, so the same code runs on the main thread
  as the fallback where worker WebGL is missing. No WebGL2 shows an "unsupported browser" message.
- Thumbnails are rendered in the browser from cached snapshots (A5.6).

### Fonts

- The canvas worker fetches the verified font assets the device loads (ADR-0032) and receives
  their bytes without copying. Platform defaults are open-licensed: Roboto for Material, Noto
  fallbacks per script.
- **SF Pro is never served or stored by Plux.** On Chromium the user's own installed copy is
  read through Local Font Access (`queryLocalFonts`, with permission), and the bytes stay in
  the browser. Elsewhere, and in Android frames, a substitute is used with the notice "device
  preview is authoritative" (`STU-006`).

## Consequences

- **Positive:** `STU-005` is a tested claim, not an impression; text layout is the device's own;
  React cannot drop a canvas frame; the port records exactly which Flutter source it follows.
- **Negative:** the layout port is the largest single piece of Studio; CanvasKit is large
  (npm default 2.80 MiB gzip) and we maintain a build; a parallel accessible DOM tree is needed;
  pixel output differs slightly from Impeller by design.
- **Go/no-go numbers:** the S0.8 prototype (`docs/benchmarks/studio-prototype.md`)
  measures frame rate, worker-to-main latency, build size and text conformance; a failed
  threshold reopens this ADR before S1 builds on it.
- **Follow-up:** the build step and size budget (S0.8, S1); the conformance harness (S1.2); the
  per-file revision check in `make studio-check`.

## Conditions of release

Decision O-7: this ADR is accepted with the following legal reviews **open**. Work merges on the
branch, but **no image is published and 0.0.1 is not cut until each is closed** (spec C3).

1. **Ported Flutter code:** BSD-3-Clause inside AGPL-3.0-only `studio/`, the notice text, and the `REUSE.toml` annotation. S1 needs this settled early because the layout port is its core.
2. **SF Pro:** the Local Font Access handling, the substitute and the notice, against Apple's licence terms.
3. **OFL-1.1 fonts:** each file's licence verified at import (Roboto, the Noto families used on the canvas).
4. **CanvasKit and Skia:** the notices that the shipped WebAssembly and its build carry.

## Options in detail

### Option 2 — PixiJS, raw WebGL2, Yoga or Taffy

PixiJS is a sprite engine with browser Canvas2D text, no faithful blur shadows and a duplicate
scene graph. Raw WebGL2 means rebuilding Skia (paths, anti-aliasing, shadows, clips, glyph atlases,
HarfBuzz, ICU, bidi). Yoga and Taffy cover only a handful of the Layer 1 widgets and implement
CSS, not Flutter, layout. Rejected.

### Option 3 — Canvas2D libraries, SVG, DOM

Too slow at scale, and browser text breaks `STU-003` and `STU-005`. Rejected.

### Option 4 — Flutter Web, Vello, ThorVG

Flutter Web ships a second runtime and no longer has the browser-text path. Vello needs WebGPU and
is young. ThorVG has no Flutter text layout. Rejected.

## Revision (2026-10-24, P11 plan M-12)

The maintainer requires the canvas to perform on computers **without a GPU**. Every canvas budget
(`STU-045`, `STU-120`, `STU-121`) holds under software rendering, and the benchmarks run in software
mode. The S0.8 prototype showed why this needs design, not tuning: 1,600 draw calls per frame took
434 ms at p95 under software GL. The canvas therefore:

- chooses its renderer at start-up by probing the GL: WebGL when the GL is hardware, CanvasKit's CPU
  raster surface when it is software (SwiftShader, or a context refused with
  `failIfMajorPerformanceCaveat`);
- keeps retained tile caches, so panning and zooming blit cached images instead of redrawing, and
  redraws only dirty regions;
- batches draws (one path per arrow style, cached text blobs) and allocates nothing per frame;
- reduces detail by zoom (no blur shadows — pre-rendered shadow images instead —, simplified arrows,
  text as bars when small) and renders at reduced resolution while the user interacts, refining when
  idle.


Measured on 2026-10-24 (`docs/benchmarks/studio-prototype.md`, "No-GPU techniques"): under
software GL, WebGL with batching, tile caches and level of detail came closest (Scene A pan p95
about 23 ms, drag p95 about 21 ms, 8–10 draw calls instead of 1,600); CanvasKit's CPU raster
surface is no faster on plain drawing and much slower once tiles are blitted (about 5 ms per image
draw), so the start-up choice keeps WebGL unless a measurement on the machine says otherwise.
16.7 ms p95 is not yet reached: S1.3 adds dirty-region redraw for drags, builds tiles outside the
frame, and is held to the budget by the software-mode benchmark.

**Software GL renders on the CPU (2026-10-25, S1.3):** on software GL the canvas renders with CanvasKit's CPU raster backend, chosen from the GL probe alone; the start-up measurement is removed, because WebGL's cost on SwiftShader lands in the GPU process after the measured work and stalls the page compositor for 0.4–0.9 s per base re-render. The base is a premultiplied raster surface whose flush copies only the dirty rectangle to a CPU-backed 2D context; the slow image draws measured earlier came from `MakeSWCanvasSurface`'s unpremultiplied pixels (`docs/benchmarks/studio.md`, S1.3).

**Settle time (2026-10-25, plan M-13):** after a pan or zoom gesture ends, every tile in the visible viewport is at full resolution within **300 ms** on a machine without a GPU; overscan tiles may finish later and stay preemptible. The base is re-rendered as budgeted, preemptible tile jobs, view first.

**Direct presentation for the app canvas (2026-11-02, maintainer: "the full canvas and full flow is not smooth").** The compositor path above glitched in use, for reasons that tuning cannot remove: the worker's pixels and the host's CSS transform reach the screen on different frames, so a re-rendered base showed for a frame at the old transform and content jumped and returned when a gesture ended; the overscan holds one tile of margin, so zooming out showed blank space until a re-render; progressive tiles filled the view over later frames; and below 50 % text became bars. A canvas may therefore choose `present: "direct"`: the canvas element is exactly the view and never transformed, and the worker redraws the whole view, without tiles, on every animation frame in which it changed — one source of truth for the pixels, so nothing jumps and nothing shows blank. `detail: "full"` keeps text, curved links and shadows at every zoom, as design tools do. The app canvas uses both, and the plugin canvas too since 2026-11-03 (the maintainer asked for the same canvas behaviour); its screens and arrows are drawn as before. What keeps a whole-view redraw within a frame: items are batched within one depth and boxes share a depth unless they overlap; on the CPU raster path a box's shadow is a batched contact ring instead of a shadow image (image draws cost that path about 2 ms a box), the grid's dots are square, and the canvas draws one device pixel per CSS pixel, as a 2x screen would quadruple the work. Measured on the CPU raster path in the test runtime (Bun's WebAssembly; the browser is expected to be no slower), a whole-view frame with 200 plugin boxes, every one in view, takes 15–27 ms median; with a GPU it is a small fraction of a frame. The WebGL surface also preserves its drawing buffer: without it the browser cleared the canvas after every frame, and the compositor path's partial draws (one tile per job, damaged rectangles) showed alone on a blank canvas, which only a GPU showed. A stroke may be dashed (`StrokeStyle.dash`, 2026-11-03): a native target on the app canvas is a dashed outline (`STU-033`), drawn with Skia's dash path effect on the stroke's batch, so dashed outlines batch like solid ones.
