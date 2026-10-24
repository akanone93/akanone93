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

- **One dedicated canvas worker** owns layout, CanvasKit, the scene graph and hit-testing, and
  draws through `transferControlToOffscreen()`, so no frame is copied. React and the document
  stay on the main thread; edits reach the worker as one JSON Patch stream (A5.1).
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

- chooses its renderer at start-up by measurement: WebGL when the GL is hardware, CanvasKit's CPU
  raster surface when it is software (SwiftShader, or a context refused with
  `failIfMajorPerformanceCaveat`);
- keeps retained tile caches, so panning and zooming blit cached images instead of redrawing, and
  redraws only dirty regions;
- batches draws (one path per arrow style, cached text blobs) and allocates nothing per frame;
- reduces detail by zoom (no blur shadows — pre-rendered shadow images instead —, simplified arrows,
  text as bars when small) and renders at reduced resolution while the user interacts, refining when
  idle.

