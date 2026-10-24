// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The read-only scene model (studio-development §9.2): a structured-clone
 * snapshot of nodes, frames, bounds, z-order and the view, for tests and the
 * accessible tree. Tests assert on it, never on pixels.
 */

import type { ItemSpec, SceneModel, SceneModelNode } from "./protocol.ts";
import type { Scene } from "./scene.ts";
import type { Viewport } from "./viewport.ts";

function labelOf(spec: ItemSpec): string {
  switch (spec.kind) {
    case "frame":
      return spec.name;
    case "text":
      return spec.text;
    case "rect":
      return spec.label ?? "";
    default:
      return "";
  }
}

function frameOf(spec: ItemSpec): string | null {
  return spec.kind === "rect" || spec.kind === "text" ? (spec.frame ?? null) : null;
}

/** A snapshot of `scene` and `viewport`. */
export function buildSceneModel(scene: Scene, viewport: Viewport): SceneModel {
  const nodes: SceneModelNode[] = [];
  const frames: string[] = [];
  for (const { spec, bounds } of scene.items()) {
    if (spec.kind === "frame") {
      frames.push(spec.id);
    }
    nodes.push({
      id: spec.id,
      kind: spec.kind,
      frame: frameOf(spec),
      label: labelOf(spec),
      bounds: { x: bounds.l, y: bounds.t, w: bounds.r - bounds.l, h: bounds.b - bounds.t },
      z: spec.z,
    });
  }
  return {
    version: scene.version,
    viewport: { x: viewport.x, y: viewport.y, zoom: viewport.zoom, width: viewport.width, height: viewport.height },
    frames,
    nodes,
  };
}
