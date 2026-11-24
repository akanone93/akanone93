// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { arrange, VIEW_ASPECT } from "../../lib/arrange.ts";
import type { DocumentDraft } from "../editor/edit-session.ts";
import type { JsonValue } from "../editor/json-patch.ts";

/**
 * The pure model of the app canvas (C1, STU-030, STU-032, STU-035): boxes, selection, zoom to fit, automatic
 * layout and the shape of the app-level `canvas` document.
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
export const BOX: Size = { width: 260, height: 152 };
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

/** The smallest side of a frame, in world units. */
export const MIN_FRAME = 80;

export interface FrameChange {
  readonly label: string;
  readonly width: number;
  readonly height: number;
}

/** The frames with frame `id` renamed and resized; a side never goes below MIN_FRAME, a blank label keeps the old. */
export function changeFrame(frames: readonly Frame[], id: string, change: FrameChange): readonly Frame[] {
  const side = (n: number, was: number) => (Number.isFinite(n) ? Math.max(MIN_FRAME, Math.round(n)) : was);
  return frames.map((f) =>
    f.id === id
      ? {
          ...f,
          label: change.label.trim() === "" ? f.label : change.label.trim(),
          width: side(change.width, f.width),
          height: side(change.height, f.height),
        }
      : f,
  );
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

/** A link between two boxes, for the automatic layout. */
export interface BoxLink {
  readonly from: string;
  readonly to: string;
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

function readPositions(list: unknown): Map<string, Point> {
  const positions = new Map<string, Point>();
  if (!Array.isArray(list)) {
    return positions;
  }
  for (const p of list as { id?: unknown; x?: unknown; y?: unknown }[]) {
    if (typeof p?.id === "string" && isNumber(p.x) && isNumber(p.y)) {
      positions.set(p.id, { x: p.x, y: p.y });
    }
  }
  return positions;
}

function readFrames(list: unknown): Frame[] {
  if (!Array.isArray(list)) {
    return [];
  }
  const frames: Frame[] = [];
  for (const f of list as FrameRow[]) {
    const ok = typeof f?.id === "string" && typeof f.label === "string";
    if (ok && isNumber(f.x) && isNumber(f.y) && isNumber(f.width) && isNumber(f.height)) {
      frames.push({ id: String(f.id), label: String(f.label), x: f.x, y: f.y, width: f.width, height: f.height });
    }
  }
  return frames;
}

/** The saved places of native route boxes (`routes`), by their box ids (`native:<route>`, #228). */
function readRoutes(list: unknown, into: Map<string, Point>): void {
  if (!Array.isArray(list)) {
    return;
  }
  for (const r of list as { route?: unknown; x?: unknown; y?: unknown }[]) {
    if (typeof r?.route === "string" && isNumber(r.x) && isNumber(r.y)) {
      into.set(`${NATIVE_BOX}${r.route}`, { x: r.x, y: r.y });
    }
  }
}

/** The saved positions (native route boxes too) and frames, or an empty canvas when the document is missing or unusable. */
export function readSavedCanvas(content: unknown): SavedCanvas {
  if (content === null || typeof content !== "object") {
    return { positions: new Map(), frames: [] };
  }
  const doc = content as { positions?: unknown; routes?: unknown; frames?: unknown };
  const positions = readPositions(doc.positions);
  readRoutes(doc.routes, positions);
  return { positions, frames: readFrames(doc.frames) };
}

/** Positions in a stable order (by ID), so the same canvas always writes the same document. */
/** The prefix of a native target's box id, `native:<route>`: a box of the canvas's own, with no document behind it. */
export const NATIVE_BOX = "native:";

/**
 * The prefix of the box of a route nothing has, `missing:<route>`, which broken links point at (#259): placed beside
 * its sources each time and never saved, as it should not last.
 */
export const MISSING_BOX = "missing:";

/**
 * The saved positions of the plugins, by document id. A native target's box has no document id (SCH-002): its place
 * is saved by route name in `routes` (#228).
 */
function positionList(positions: ReadonlyMap<string, Point>): JsonValue[] {
  return [...positions.entries()]
    .filter(([id]) => !id.startsWith(NATIVE_BOX) && !id.startsWith(MISSING_BOX))
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([id, p]) => ({ id, x: Math.round(p.x), y: Math.round(p.y) }));
}

/** The saved places of the native route boxes, by route name, in route order (#228). */
function routeList(positions: ReadonlyMap<string, Point>): JsonValue[] {
  return [...positions.entries()]
    .filter(([id]) => id.startsWith(NATIVE_BOX))
    .map(([id, p]) => ({ route: id.slice(NATIVE_BOX.length), x: Math.round(p.x), y: Math.round(p.y) }))
    .sort((a, b) => (a.route < b.route ? -1 : a.route > b.route ? 1 : 0));
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
    routes: routeList(saved.positions),
    frames: frameList(saved.frames),
  };
}

/** The edit recipe that stores positions and frames in an existing canvas document. */
export function canvasRecipe(saved: SavedCanvas): (draft: DocumentDraft) => void {
  return (draft) => {
    Object.assign(draft, {
      positions: positionList(saved.positions),
      routes: routeList(saved.positions),
      frames: frameList(saved.frames),
    });
  };
}

/**
 * The automatic layout (STU-032, #241): a layered layout per connected group of plugins, from the entry plugin
 * outwards, wrapped and packed towards the view's shape (`arrange`). Gaps scale with the box: wide enough between
 * layers for the arrows to run round the boxes.
 */
export function autoLayout(
  ids: readonly string[],
  links: readonly BoxLink[],
  size: Size = BOX,
  entry?: string,
  aspect: number = VIEW_ASPECT,
): ReadonlyMap<string, Point> {
  const gapX = Math.round(size.width * 0.46);
  const gapY = Math.round(size.height * 0.42);
  return arrange(
    ids.map((id) => ({ id, w: size.width, h: size.height })),
    links,
    { gapX, gapY, groupGap: Math.round(gapX * 1.34), aspect, entry },
  );
}

/** Saved positions where there are some; the automatic layout for every other plugin. */
export function placeBoxes(
  plugins: readonly { readonly id: string; readonly key: string }[],
  saved: ReadonlyMap<string, Point>,
  links: readonly BoxLink[],
  entry?: string,
): PlacedBox[] {
  const missing = plugins.filter((p) => !saved.has(p.id));
  const laid =
    missing.length === 0
      ? new Map<string, Point>()
      : autoLayout(
          plugins.map((p) => p.id),
          links,
          BOX,
          entry,
        );
  return plugins.map((p) => {
    const at = saved.get(p.id) ?? laid.get(p.id) ?? { x: 0, y: 0 };
    return { id: p.id, key: p.key, x: at.x, y: at.y };
  });
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

/** The least zoom the opening view goes down to: below it a plugin's title is too small to read (#205). */
export const COMFORT_ZOOM = 0.35;

/**
 * The opening view (#205, ADR-0103 as amended 2026-11-22): every box in view, with room around them, at most 100 %;
 * when that would take the view below `COMFORT_ZOOM`, as many as fit at that zoom, centred on the boxes' middle.
 */
export function openingViewport(boxes: readonly Point[], view: Size, size: Size = BOX, margin = 72): Viewport {
  if (boxes.length === 0 || view.width <= 0 || view.height <= 0) {
    return { x: -view.width / 2, y: -view.height / 2, z: 1 };
  }
  const minX = Math.min(...boxes.map((b) => b.x));
  const minY = Math.min(...boxes.map((b) => b.y));
  const maxX = Math.max(...boxes.map((b) => b.x + size.width));
  const maxY = Math.max(...boxes.map((b) => b.y + size.height));
  const fits = Math.min((view.width - 2 * margin) / (maxX - minX), (view.height - 2 * margin) / (maxY - minY));
  const z = Math.min(1, Math.max(COMFORT_ZOOM, fits));
  return { z, x: (minX + maxX) / 2 - view.width / 2 / z, y: (minY + maxY) / 2 - view.height / 2 / z };
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
