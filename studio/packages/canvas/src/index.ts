// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * `@plux/canvas`: the renderer core of Plux Canvas (STU-003, STU-045,
 * STU-120, STU-121, ADR-0013 M-12), built first for machines without a GPU.
 */

export {
  type A11yDocumentLike,
  type A11yElementLike,
  type A11yStyleLike,
  AccessibleTree,
  ROLES,
} from "./a11y.ts";
export { Batches, BatchLayer, color4f } from "./batch.ts";
export { type BrowserScope, browserWorkerDeps, watchCanvasContext } from "./browser.ts";
export {
  AUTHORITATIVE_NOTICE,
  type CanvasNotice,
  complexScriptOf,
  type FontLoadDeps,
  type FontRequest,
  type FontSource,
  type LoadedFonts,
  type LocalFontLike,
  loadCanvasFonts,
  type NoticeReason,
  SF_PRO_FAMILY,
} from "./fonts.ts";
export { intersects, makeRect, type Rect } from "./geometry.ts";
export { HIT_CELL, HitIndex, MAX_CELLS, type RectMode, shapeOf } from "./hit.ts";
export {
  type CanvasElementLike,
  CanvasHost,
  type CanvasSize,
  HIT_TOLERANCE,
  type HostDeps,
  type HostEvent,
  type HostLayers,
  type WorkerLike,
} from "./host.ts";
export {
  INTERACTION_RESOLUTION,
  type Lod,
  lodForZoom,
  resolutionFor,
  SHADOWS_BELOW,
  STRAIGHT_LINKS_BELOW,
  TEXT_BARS_BELOW,
} from "./lod.ts";
export {
  type Context2DLike,
  type DeviceRect,
  HANDLE_SIZE,
  OUTLINE_WIDTH,
  type OverlayCanvasLike,
  type OverlayColors,
  type OverlayView,
  type SelectionMarks,
  SelectionOverlay,
  selectionMarks,
} from "./overlay.ts";
export * from "./protocol.ts";
export { MAX_REGIONS, type RendererOptions, SceneRenderer } from "./renderer.ts";
export { type Deletable, Resources } from "./resources.ts";
export { boundsOf, moveSpec, Scene, type SceneItem } from "./scene.ts";
export { buildSceneModel } from "./scene-model.ts";
export { frameId, layoutScreens, overflowId } from "./screens.ts";
export { chooseRenderer, type GlProbe, isSoftwareGl } from "./select.ts";
export { DEFAULT_TILE_SIZE, RETILE_THRESHOLD, type Tile, TileCache } from "./tiles.ts";
export { clampZoom, MAX_ZOOM, MIN_ZOOM, Viewport } from "./viewport.ts";
export {
  CanvasWorkerCore,
  canvasParagraphEngine,
  type DisposableEngine,
  fallbackFamilies,
  IDLE_BUDGET_MS,
  type SurfaceHandlers,
  serveCanvasWorker,
  type WorkerDeps,
  type WorkerScopeLike,
} from "./worker.ts";
