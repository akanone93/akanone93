// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Ghost screens and native nodes of the plugin canvas (STU-052, STU-054): the screens of other plugins
 * and the native routes that this plugin navigates to, with the arrows that lead to them (STU-051).
 * Pure model: it decides what exists, where it sits and which scene items draw it.
 */

import type { ArrowBox, ColorRgba, ItemSpec, NavigateEdge, SceneOp } from "@plux/canvas";
import { firstText, type Node, parseDocument } from "./plugin-model.ts";
import type { Point } from "./positions.ts";
import { DEVICE } from "./scene.ts";

/** A screen of any plugin of the app, found by its route. */
export interface AppScreen {
  readonly id: string;
  readonly route: string;
  readonly title: string;
  readonly pluginId: string;
}

interface DocLike {
  readonly pluginId: string;
  readonly kind: string;
  readonly content: Uint8Array;
}

/** The screens (pages with a route) of every plugin in `documents`. */
export function readAppScreens(documents: readonly DocLike[]): AppScreen[] {
  const out: AppScreen[] = [];
  for (const d of documents) {
    if (d.kind !== "page") {
      continue;
    }
    const p = parseDocument(d.content) as
      | { id?: unknown; route?: unknown; title?: unknown; pageKind?: unknown; root?: Node }
      | undefined;
    if (p === undefined || p === null || typeof p.id !== "string" || typeof p.route !== "string") {
      continue;
    }
    if (p.pageKind !== undefined && p.pageKind !== "screen") {
      continue;
    }
    out.push({
      id: p.id,
      route: p.route,
      title: firstText(p.root) ?? (typeof p.title === "string" ? p.title : p.route),
      pluginId: d.pluginId,
    });
  }
  return out;
}

/** What the plugin's screens navigate to outside the plugin. */
export interface GhostTarget {
  /** The arrow box ID: `ghost:<screen id>` or `native:<route>`. */
  readonly id: string;
  readonly kind: "screen" | "native";
  readonly title: string;
  readonly route: string;
  /** The plugin that owns a ghost screen; undefined for a native route. */
  readonly pluginId?: string;
  /** The ghost screen's own ID; undefined for a native route. */
  readonly screenId?: string;
  /** How many navigate actions of this plugin lead here. */
  readonly links: number;
}

export interface OwnScreen {
  readonly id: string;
  readonly route: string;
  readonly title: string;
  /** Routes this screen navigates to, in document order. */
  readonly targets: readonly string[];
}

export interface GhostGraph {
  readonly targets: readonly GhostTarget[];
  /** Every navigate action that leaves a screen of the plugin, to an own screen, a ghost or a native route. */
  readonly edges: readonly NavigateEdge[];
}

/**
 * Resolves the routes the screens of `plugin` navigate to (STU-051, STU-052, STU-054). A route is an own
 * screen, a screen of another plugin (a ghost), a native route, or unknown (no box, no arrow).
 */
export function deriveGhosts(input: {
  readonly plugin: string;
  readonly screens: readonly OwnScreen[];
  readonly appScreens: readonly AppScreen[];
  readonly nativeRoutes: readonly string[];
}): GhostGraph {
  const own = new Map(input.screens.map((s) => [s.route, s]));
  const foreign = new Map<string, AppScreen>();
  for (const s of input.appScreens) {
    if (s.pluginId !== input.plugin && !foreign.has(s.route)) {
      foreign.set(s.route, s);
    }
  }
  const native = new Set(input.nativeRoutes);
  const targets = new Map<string, GhostTarget>();
  const edges: NavigateEdge[] = [];
  for (const s of input.screens) {
    s.targets.forEach((route, i) => {
      const mine = own.get(route);
      if (mine !== undefined) {
        edges.push(edge(s, i, mine.id, mine.title));
        return;
      }
      const target = outsideTarget(route, foreign.get(route), native);
      if (target === undefined) {
        return;
      }
      targets.set(target.id, { ...target, links: (targets.get(target.id)?.links ?? 0) + 1 });
      edges.push(edge(s, i, target.id, target.title));
    });
  }
  return { targets: [...targets.values()], edges };
}

/** The ghost screen of another plugin, or the native node, that `route` leads to; undefined for an unknown route. */
function outsideTarget(
  route: string,
  other: AppScreen | undefined,
  native: ReadonlySet<string>,
): GhostTarget | undefined {
  if (other !== undefined) {
    return {
      id: `ghost:${other.id}`,
      kind: "screen",
      title: other.title,
      route,
      pluginId: other.pluginId,
      screenId: other.id,
      links: 0,
    };
  }
  return native.has(route) ? { id: `native:${route}`, kind: "native", title: route, route, links: 0 } : undefined;
}

function edge(s: OwnScreen, index: number, target: string, targetTitle: string): NavigateEdge {
  return { id: `${s.id}#${index}`, source: s.id, target, trigger: "navigate", title: `${s.title} → ${targetTitle}` };
}

/** Ghost cards are the device frame at half size; a native node is a short card. */
export const GHOST = { w: DEVICE.width / 2, h: DEVICE.height / 2, nativeH: 72, gap: 48, offset: 240 } as const;

/** A target and the rectangle it occupies on the canvas. */
export interface PlacedGhost extends GhostTarget, ArrowBox {}

/** Stacks the targets in one column right of the plugin's screens (`right`), starting at `top`. */
export function placeGhosts(targets: readonly GhostTarget[], right: number, top: number): PlacedGhost[] {
  let y = top;
  return targets.map((t) => {
    const h = t.kind === "screen" ? GHOST.h : GHOST.nativeH;
    const placed = { ...t, x: right + GHOST.offset, y, w: GHOST.w, h };
    y += h + GHOST.gap;
    return placed;
  });
}

/** The colours ghosts are drawn in. */
export interface GhostStyle {
  readonly stroke: ColorRgba;
  readonly fill: ColorRgba;
  readonly nativeFill: ColorRgba;
  readonly text: ColorRgba;
  readonly pulse: ColorRgba;
}

/** Dash and gap of the dashed frame, in world units. */
const DASH = 10;
const GAP = 7;
const Z = 2;

/** Segments of the dashed outline of a rectangle, clockwise from its top-left corner. */
export function dashedOutline(x: number, y: number, w: number, h: number): { from: Point; to: Point }[] {
  const corners: Point[] = [
    { x, y },
    { x: x + w, y },
    { x: x + w, y: y + h },
    { x, y: y + h },
  ];
  const out: { from: Point; to: Point }[] = [];
  for (let side = 0; side < 4; side++) {
    const a = corners[side] as Point;
    const b = corners[(side + 1) % 4] as Point;
    const len = Math.hypot(b.x - a.x, b.y - a.y);
    for (let at = 0; at < len; at += DASH + GAP) {
      const f0 = at / len;
      const f1 = Math.min(at + DASH, len) / len;
      out.push({
        from: { x: a.x + (b.x - a.x) * f0, y: a.y + (b.y - a.y) * f0 },
        to: { x: a.x + (b.x - a.x) * f1, y: a.y + (b.y - a.y) * f1 },
      });
    }
  }
  return out;
}

/** The labels a ghost shows. */
export interface GhostLabels {
  readonly links: (count: number) => string;
}

/** The scene items that draw `ghost`: card (hit target), thumbnail, dashed frame, title and link count. */
export function ghostItems(
  ghost: PlacedGhost,
  style: GhostStyle,
  labels: GhostLabels,
  image: string | undefined,
): ItemSpec[] {
  const { x, y, w, h } = ghost;
  const items: ItemSpec[] = [
    {
      kind: "rect",
      id: ghostItem(ghost.id),
      label: ghost.title,
      x,
      y,
      w,
      h,
      radius: 8,
      fill: ghost.kind === "screen" ? style.fill : style.nativeFill,
      ...(image !== undefined ? { image } : {}),
      z: Z,
    },
    ...dashedOutline(x, y, w, h).map(
      (s, i): ItemSpec => ({
        kind: "link",
        id: `${ghostItem(ghost.id)}/d${i}`,
        from: s.from,
        to: s.to,
        stroke: { color: style.stroke, width: 1.5 },
        z: Z + 1,
      }),
    ),
    {
      kind: "text",
      id: `${ghostItem(ghost.id)}/title`,
      x: x + 12,
      y: y + 10,
      w: w - 24,
      text: ghost.title,
      size: 14,
      color: style.text,
      z: Z + 2,
    },
    {
      kind: "text",
      id: `${ghostItem(ghost.id)}/links`,
      x: x + 12,
      y: y + h - 26,
      w: w - 24,
      text: labels.links(ghost.links),
      size: 12,
      color: style.text,
      z: Z + 2,
    },
  ];
  return items;
}

/** The ID of the main scene item of a ghost; its parts are `<id>/…`. */
export const ghostItem = (boxId: string): string => `ghost-node/${boxId}`;

/** The ghost a hit item belongs to, or undefined for any other item. */
export function ghostOfHit(hits: readonly string[], ghosts: readonly GhostTarget[]): GhostTarget | undefined {
  for (const hit of hits) {
    const found = ghosts.find((g) => hit === ghostItem(g.id) || hit.startsWith(`${ghostItem(g.id)}/`));
    if (found !== undefined) {
      return found;
    }
  }
  return undefined;
}

/**
 * Keeps what the scene holds for a set of items and turns the next set into the smallest patch:
 * add the new, replace the changed, remove the gone.
 */
export class SceneSync {
  private held = new Map<string, string>();

  sync(items: readonly ItemSpec[]): SceneOp[] {
    const ops: SceneOp[] = [];
    const next = new Map<string, string>();
    for (const item of items) {
      const key = JSON.stringify(item);
      next.set(item.id, key);
      const before = this.held.get(item.id);
      if (before === undefined) {
        ops.push({ op: "add", item });
      } else if (before !== key) {
        ops.push({ op: "replace", item });
      }
    }
    for (const id of this.held.keys()) {
      if (!next.has(id)) {
        ops.push({ op: "remove", id });
      }
    }
    this.held = next;
    return ops;
  }
}

/** The 2 s pulse around a screen (STU-053): four blinks of a ring, then gone. */
export const PULSE_MS = 2000;

export function pulseSteps(
  id: string,
  rect: { l: number; t: number; r: number; b: number },
  color: ColorRgba,
): { at: number; ops: SceneOp[] }[] {
  const ring = (width: number): ItemSpec => ({
    kind: "rect",
    id,
    x: rect.l - 6,
    y: rect.t - 6,
    w: rect.r - rect.l + 12,
    h: rect.b - rect.t + 12,
    radius: 14,
    stroke: { color, width },
    z: 50,
  });
  const steps: { at: number; ops: SceneOp[] }[] = [{ at: 0, ops: [{ op: "add", item: ring(6) }] }];
  for (let i = 1; i < 4; i++) {
    steps.push({ at: (PULSE_MS / 4) * i, ops: [{ op: "replace", item: ring(i % 2 === 1 ? 2 : 6) }] });
  }
  steps.push({ at: PULSE_MS, ops: [{ op: "remove", id }] });
  return steps;
}
