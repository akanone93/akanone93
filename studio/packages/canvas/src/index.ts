// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * `@plux/canvas`: the renderer core of Plux Canvas (STU-003, STU-045,
 * STU-120, STU-121, ADR-0013 M-12), built first for machines without a GPU.
 */

export { Batches, BatchLayer, color4f } from "./batch.ts";
export { type BrowserScope, browserWorkerDeps } from "./browser.ts";
export { intersects, makeRect, type Rect } from "./geometry.ts";
export { type CanvasElementLike, CanvasHost, type CanvasSize, type HostDeps, type WorkerLike } from "./host.ts";
export {
  INTERACTION_RESOLUTION,
  type Lod,
  lodForZoom,
  resolutionFor,
  SHADOWS_BELOW,
  STRAIGHT_LINKS_BELOW,
  TEXT_BARS_BELOW,
} from "./lod.ts";
export * from "./protocol.ts";
export { MAX_REGIONS, type RendererOptions, SceneRenderer } from "./renderer.ts";
export { type Deletable, Resources } from "./resources.ts";
export { boundsOf, moveSpec, Scene, type SceneItem } from "./scene.ts";
export { buildSceneModel } from "./scene-model.ts";
export { chooseRenderer, type GlProbe, isSoftwareGl, measureSurface, RASTER_ADVANTAGE } from "./select.ts";
export { DEFAULT_TILE_SIZE, RETILE_THRESHOLD, type Tile, TileCache } from "./tiles.ts";
export { clampZoom, MAX_ZOOM, MIN_ZOOM, Viewport } from "./viewport.ts";
export {
  CanvasWorkerCore,
  IDLE_BUDGET_MS,
  serveCanvasWorker,
  type WorkerDeps,
  type WorkerScopeLike,
} from "./worker.ts";
