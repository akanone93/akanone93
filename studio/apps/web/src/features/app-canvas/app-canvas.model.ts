// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import dagre from "dagre";
import type { DocumentDraft } from "../editor/edit-session.ts";
import type { JsonValue } from "../editor/json-patch.ts";

/**
 * The pure model of the app canvas (C1, STU-030, STU-032, STU-035): boxes, snapping with guides,
 * selection, zoom to fit, automatic layout and the shape of the app-level `canvas` document.
 * Nothing here touches React, the network or the clock.
 */

export interface Point {
  readonly x: number;
  readonly y: number;
}

export interface Size {
  readonly width: number;
  readonly height: number;
}

/** World size of a plugin box. */
export const BOX: Size = { width: 224, height: 112 };
/** Distance in world units within which a box snaps to another box's edge or centre. */
export const SNAP_DISTANCE = 6;
export const MIN_ZOOM = 0.1;
export const MAX_ZOOM = 4;

/** The path of the app-level canvas document. */
export const APP_CANVAS_PATH = "canvas.json";

export interface Frame {
  readonly id: string;
  readonly label: string;
  readonly x: number;
  readonly y: number;
  readonly width: number;
  readonly height: number;
}

export interface SavedCanvas {
  readonly positions: ReadonlyMap<string, Point>;
  readonly frames: readonly Frame[];
}

/** A plugin placed on the canvas, with its summary as the badges show it. */
export interface PlacedBox extends Point {
  readonly id: string;
  readonly key: string;
}

/** A link between two boxes; the typed hook where the routing step draws arrows. */
export interface BoxLink {
  readonly from: string;
  readonly to: string;
}

/** What an arrow layer receives; the routing step supplies the component (S3.2). */
export interface ArrowLayerProps {
  readonly boxes: readonly PlacedBox[];
  readonly links: readonly BoxLink[];
  readonly boxSize: Size;
}

interface FrameRow {
  id?: unknown;
  label?: unknown;
  x?: unknown;
  y?: unknown;
  width?: unknown;
  height?: unknown;
}

const isNumber = (v: unknown): v is number => typeof v === "number" && Number.isFinite(v);

/** The saved positions and frames, or an empty canvas when the document is missing or unusable. */
export function readSavedCanvas(content: unknown): SavedCanvas {
  const positions = new Map<string, Point>();
  const frames: Frame[] = [];
  if (content === null || typeof content !== "object") {
    return { positions, frames };
  }
  const doc = content as { positions?: unknown; frames?: unknown };
  if (Array.isArray(doc.positions)) {
    for (const p of doc.positions as { id?: unknown; x?: unknown; y?: unknown }[]) {
      if (typeof p?.id === "string" && isNumber(p.x) && isNumber(p.y)) {
        positions.set(p.id, { x: p.x, y: p.y });
      }
    }
  }
  if (Array.isArray(doc.frames)) {
    for (const f of doc.frames as FrameRow[]) {
      if (
        typeof f?.id === "string" &&
        typeof f.label === "string" &&
        isNumber(f.x) &&
        isNumber(f.y) &&
        isNumber(f.width) &&
        isNumber(f.height)
      ) {
        frames.push({ id: f.id, label: f.label, x: f.x, y: f.y, width: f.width, height: f.height });
      }
    }
  }
  return { positions, frames };
}

/** Positions in a stable order (by ID), so the same canvas always writes the same document. */
function positionList(positions: ReadonlyMap<string, Point>): JsonValue[] {
  return [...positions.entries()]
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([id, p]) => ({ id, x: Math.round(p.x), y: Math.round(p.y) }));
}

function frameList(frames: readonly Frame[]): JsonValue[] {
  return frames.map((f) => ({
    id: f.id,
    label: f.label,
    x: Math.round(f.x),
    y: Math.round(f.y),
    width: Math.round(f.width),
    height: Math.round(f.height),
  }));
}

/** A new app canvas document (STU-032). */
export function canvasDocument(id: string, saved: SavedCanvas): JsonValue {
  return {
    schemaVersion: "1.0.0",
    kind: "canvas",
    id,
    positions: positionList(saved.positions),
    frames: frameList(saved.frames),
  };
}

/** The edit recipe that stores positions and frames in an existing canvas document. */
export function canvasRecipe(saved: SavedCanvas): (draft: DocumentDraft) => void {
  return (draft) => {
    draft["positions"] = positionList(saved.positions);
    draft["frames"] = frameList(saved.frames);
  };
}

/** Dagre layout, left to right, for plugins that have no saved position (STU-032). */
export function autoLayout(
  ids: readonly string[],
  links: readonly BoxLink[],
  size: Size = BOX,
): ReadonlyMap<string, Point> {
  const g = new dagre.graphlib.Graph();
  g.setGraph({ rankdir: "LR", nodesep: 48, ranksep: 96, marginx: 0, marginy: 0 });
  g.setDefaultEdgeLabel(() => ({}));
  for (const id of ids) {
    g.setNode(id, { width: size.width, height: size.height });
  }
  const known = new Set(ids);
  for (const l of links) {
    if (known.has(l.from) && known.has(l.to) && l.from !== l.to) {
      g.setEdge(l.from, l.to);
    }
  }
  dagre.layout(g);
  const out = new Map<string, Point>();
  for (const id of ids) {
    const n = g.node(id);
    out.set(id, { x: Math.round(n.x - size.width / 2), y: Math.round(n.y - size.height / 2) });
  }
  return out;
}

/** Saved positions where there are some; the automatic layout for every other plugin. */
export function placeBoxes(
  plugins: readonly { readonly id: string; readonly key: string }[],
  saved: ReadonlyMap<string, Point>,
  links: readonly BoxLink[],
): PlacedBox[] {
  const missing = plugins.filter((p) => !saved.has(p.id));
  const laid =
    missing.length === 0
      ? new Map<string, Point>()
      : autoLayout(
          plugins.map((p) => p.id),
          links,
        );
  return plugins.map((p) => {
    const at = saved.get(p.id) ?? laid.get(p.id) ?? { x: 0, y: 0 };
    return { id: p.id, key: p.key, x: at.x, y: at.y };
  });
}

export interface Guide {
  readonly axis: "x" | "y";
  /** World coordinate of the guide line. */
  readonly at: number;
}

export interface Snapped {
  readonly point: Point;
  readonly guides: readonly Guide[];
}

/**
 * Snaps a box being dragged to the edges and centres of the other boxes; returns the guides to draw
 * through `CanvasHost.setGuides`-style overlays. Each axis snaps to the nearest candidate in range.
 */
export function snap(
  moving: Point,
  others: readonly Point[],
  size: Size = BOX,
  distance: number = SNAP_DISTANCE,
): Snapped {
  const axis = (value: number, extent: number, pick: (o: Point) => number, kind: "x" | "y") => {
    let best: { delta: number; at: number } | undefined;
    for (const o of others) {
      const base = pick(o);
      for (const target of [base, base + extent / 2, base + extent]) {
        for (const mine of [0, extent / 2, extent]) {
          const delta = target - (value + mine);
          if (Math.abs(delta) <= distance && (best === undefined || Math.abs(delta) < Math.abs(best.delta))) {
            best = { delta, at: target };
          }
        }
      }
    }
    return best === undefined ? undefined : { value: value + best.delta, guide: { axis: kind, at: best.at } as Guide };
  };
  const sx = axis(moving.x, size.width, (o) => o.x, "x");
  const sy = axis(moving.y, size.height, (o) => o.y, "y");
  return {
    point: { x: sx?.value ?? moving.x, y: sy?.value ?? moving.y },
    guides: [sx?.guide, sy?.guide].filter((g): g is Guide => g !== undefined),
  };
}

/** Click selects one; with `additive` it toggles; the result is a new set. */
export function select(current: ReadonlySet<string>, id: string, additive: boolean): ReadonlySet<string> {
  if (!additive) {
    return new Set([id]);
  }
  const next = new Set(current);
  if (!next.delete(id)) {
    next.add(id);
  }
  return next;
}

export interface Rect {
  readonly x: number;
  readonly y: number;
  readonly width: number;
  readonly height: number;
}

/** The boxes a marquee touches. */
export function marquee(boxes: readonly PlacedBox[], rect: Rect, size: Size = BOX): ReadonlySet<string> {
  return new Set(
    boxes
      .filter(
        (b) =>
          b.x < rect.x + rect.width &&
          b.x + size.width > rect.x &&
          b.y < rect.y + rect.height &&
          b.y + size.height > rect.y,
      )
      .map((b) => b.id),
  );
}

export interface Viewport {
  readonly x: number;
  readonly y: number;
  readonly z: number;
}

/** The viewport that shows every box with a margin (zoom to fit); the origin at zoom 1 when empty. */
export function fitViewport(boxes: readonly Point[], view: Size, size: Size = BOX, margin = 64): Viewport {
  if (boxes.length === 0 || view.width <= 0 || view.height <= 0) {
    return { x: 0, y: 0, z: 1 };
  }
  const minX = Math.min(...boxes.map((b) => b.x));
  const minY = Math.min(...boxes.map((b) => b.y));
  const maxX = Math.max(...boxes.map((b) => b.x + size.width));
  const maxY = Math.max(...boxes.map((b) => b.y + size.height));
  const z = Math.min(
    MAX_ZOOM,
    Math.max(MIN_ZOOM, Math.min((view.width - 2 * margin) / (maxX - minX), (view.height - 2 * margin) / (maxY - minY))),
  );
  return {
    z,
    x: (minX + maxX) / 2 - view.width / 2 / z,
    y: (minY + maxY) / 2 - view.height / 2 / z,
  };
}

/** The key of a new plugin from its name: lower-kebab, starting with a letter (SCH-005). */
export function pluginKeyFrom(name: string, taken: ReadonlySet<string>): string {
  const base =
    name
      .toLowerCase()
      .normalize("NFKD")
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "")
      .replace(/^[0-9-]+/, "")
      .slice(0, 56) || "plugin";
  let key = base;
  for (let n = 2; taken.has(key); n++) {
    key = `${base}-${n}`;
  }
  return key;
}
