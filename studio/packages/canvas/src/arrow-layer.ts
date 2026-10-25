// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Arrows as scene items (spec §21.5, STU-041, STU-044 to STU-046, STU-048).
 * `ArrowCanvas` turns the router's routes into link, rect and text items and
 * emits only the scene patches that changed, so the renderer draws all arrows
 * of one style in one batched path (ADR-0013 M-12) and the main-thread hit
 * index resolves clicks: `pick` maps the ids it returns back to a bundle or a
 * single lane. The app calls `moveBox` while dragging, `setFocus` on hover or
 * keyboard focus, `frame` from its frame loop while `animating`, and
 * `setZoom` when the zoom crosses the detail threshold.
 */

import type { ArrowRouter, Bundle, Cubic, NavigateEdge, Route } from "./arrows.ts";
import { sideNormal } from "./arrows.ts";
import {
  type ArrowLod,
  arrowLodForZoom,
  Expansion,
  type LaneMenuItem,
  laneGeometry,
  laneMenu,
  routeMidpoint,
} from "./lanes.ts";
import type { ColorRgba, ItemSpec, Point, SceneOp, StrokeStyle } from "./protocol.ts";

/** Colours, widths and depth of arrows. */
export interface ArrowStyle {
  readonly stroke: StrokeStyle;
  /** Bundles unrelated to the expanded one (STU-044). */
  readonly dimmed: StrokeStyle;
  /** The expanded bundle's lanes. */
  readonly focus: StrokeStyle;
  readonly badgeFill: ColorRgba;
  readonly badgeText: ColorRgba;
  readonly labelText: ColorRgba;
  /** Depth of collapsed arrows; the expanded bundle is raised by 10. */
  readonly z: number;
}

/** Default arrow style; the app passes its Porcelain tokens. */
export const defaultArrowStyle: ArrowStyle = Object.freeze({
  stroke: { color: 0x5f6368ff, width: 1.5 },
  dimmed: { color: 0x5f636840, width: 1.5 },
  focus: { color: 0x1a73e8ff, width: 2 },
  badgeFill: 0x5f6368ff,
  badgeText: 0xffffffff,
  labelText: 0x202124ff,
  z: 0,
});

/** What a click on an arrow item selects: a bundle, or one lane's link (STU-046). */
export interface ArrowPick {
  readonly bundle: Bundle;
  readonly link?: NavigateEdge;
}

const HEAD = 8;
const HEAD_ANGLE = 0.45;
const BADGE_W = 22;
const BADGE_H = 16;
const LABEL_W = 160;
const LABEL_SIZE = 11;

/** An item and the lane link it belongs to, if any. */
interface Out {
  readonly item: ItemSpec;
  readonly link: NavigateEdge | undefined;
}

interface Owner {
  readonly bundle: Bundle;
  readonly link: NavigateEdge | undefined;
}

function link(id: string, c: Cubic, stroke: StrokeStyle, z: number, straight: boolean): ItemSpec {
  return straight
    ? { kind: "link", id, from: c.from, to: c.to, stroke, z }
    : { kind: "link", id, from: c.from, c1: c.c1, c2: c.c2, to: c.to, stroke, z };
}

function segment(id: string, from: Point, to: Point, stroke: StrokeStyle, z: number): ItemSpec {
  return { kind: "link", id, from, to, stroke, z };
}

/** Scene items and patches for the arrows of one canvas. */
export class ArrowCanvas {
  readonly expansion = new Expansion();
  private lod: ArrowLod;
  private moving: string[] = [];
  private readonly items = new Map<string, string[]>();
  private readonly owners = new Map<string, Owner>();

  constructor(
    readonly router: ArrowRouter,
    private readonly style: ArrowStyle = defaultArrowStyle,
    zoom = 1,
  ) {
    this.lod = arrowLodForZoom(zoom);
  }

  /** Patches drawing every bundle (first draw, or after `router.setGraph`). */
  render(now = 0): SceneOp[] {
    const ops: SceneOp[] = [];
    const live = this.router.routes;
    for (const id of [...this.items.keys()]) {
      if (!live.has(id)) {
        this.drop(id, ops);
      }
    }
    for (const route of live.values()) {
      this.emit(route, now, ops);
    }
    return ops;
  }

  /** Sets the zoom; redraws everything only when the level of detail changes (STU-048). */
  setZoom(zoom: number, now = 0): SceneOp[] {
    const next = arrowLodForZoom(zoom);
    if (next === this.lod) {
      return [];
    }
    this.lod = next;
    return this.render(now);
  }

  /** Moves box `id` and returns the patches for the re-routed bundles (STU-045). */
  moveBox(id: string, x: number, y: number, now = 0): SceneOp[] {
    const ops: SceneOp[] = [];
    for (const bundle of this.router.moveBox(id, x, y)) {
      const route = this.router.routes.get(bundle);
      if (route) {
        this.emit(route, now, ops);
      }
    }
    return ops;
  }

  /**
   * Expands bundle `id` on hover or keyboard focus, or collapses all (null).
   * Unrelated bundles dim and the expanded one is raised (STU-044), so every
   * bundle is restyled once here; the animation itself runs in `frame`.
   */
  setFocus(id: string | null, now: number): SceneOp[] {
    const changed = this.expansion.setFocus(id, now);
    if (changed.length === 0) {
      return [];
    }
    this.moving = [...new Set([...this.moving, ...changed])];
    return this.render(now);
  }

  /** Whether a bundle is still expanding or collapsing at `now`. */
  animating(now: number): boolean {
    return this.expansion.animating(now).length > 0;
  }

  /**
   * Per-frame patches while animating: the interpolated lanes of the moving
   * bundles, plus one last patch for each bundle whose animation just ended.
   */
  frame(now: number): SceneOp[] {
    const ops: SceneOp[] = [];
    const moving = this.expansion.animating(now);
    const ids = new Set([...this.moving, ...moving]);
    for (const id of [...ids].sort()) {
      const route = this.router.routes.get(id);
      if (route) {
        this.emit(route, now, ops);
      }
    }
    this.moving = moving;
    this.expansion.settle(now);
    return ops;
  }

  /**
   * The arrow under a click, from the hit index's ids (topmost first): a
   * lane's link when the bundle is expanded, else the bundle (STU-046).
   */
  pick(ids: readonly string[]): ArrowPick | undefined {
    for (const id of ids) {
      const owner = this.owners.get(id);
      if (owner) {
        return owner.link ? { bundle: owner.bundle, link: owner.link } : { bundle: owner.bundle };
      }
    }
    return undefined;
  }

  /** The lane menu for a picked lane (STU-046); empty for a whole bundle. */
  menu(pick: ArrowPick): LaneMenuItem[] {
    return pick.link ? laneMenu(pick.link) : [];
  }

  private drop(id: string, ops: SceneOp[]): void {
    for (const item of this.items.get(id) ?? []) {
      ops.push({ op: "remove", id: item });
      this.owners.delete(item);
    }
    this.items.delete(id);
  }

  private emit(route: Route, now: number, ops: SceneOp[]): void {
    const { bundle } = route;
    const next = this.build(route, now);
    const previous = this.items.get(bundle.id) ?? [];
    const ids: string[] = [];
    const seen = new Set<string>();
    for (const { item, link: owner } of next) {
      ops.push({ op: "replace", item });
      ids.push(item.id);
      seen.add(item.id);
      this.owners.set(item.id, { bundle, link: owner });
    }
    for (const id of previous) {
      if (!seen.has(id)) {
        ops.push({ op: "remove", id });
        this.owners.delete(id);
      }
    }
    this.items.set(bundle.id, ids);
  }

  private build(route: Route, now: number): Out[] {
    const { bundle } = route;
    const style = this.style;
    const focus = this.expansion.focus;
    const progress = this.lod.lanes ? this.expansion.progress(bundle.id, now) : 0;
    const raised = progress > 0 || focus === bundle.id;
    const stroke = raised ? style.focus : focus !== null ? style.dimmed : style.stroke;
    const z = raised ? style.z + 10 : style.z;
    const prefix = `arrow:${bundle.id}:`;
    const out: Out[] = [];
    const first = route.segments[0] as Cubic;
    const last = route.segments.at(-1) as Cubic;
    if (this.lod.straight) {
      out.push({ item: segment(`${prefix}s0`, first.from, last.to, stroke, z), link: undefined });
    } else if (progress > 0) {
      this.lanes(route, progress, prefix, stroke, z, out);
    } else {
      route.segments.forEach((c, k) => {
        out.push({ item: link(`${prefix}s${k}`, c, stroke, z, false), link: undefined });
      });
    }
    head(route, prefix, stroke, z, out);
    if (this.lod.labels && progress === 0 && bundle.links.length > 1) {
      const fill = raised || focus === null ? style.badgeFill : style.dimmed.color;
      this.badge(route, prefix, fill, z, out);
    }
    return out;
  }

  /** The lanes of an expanding bundle, titled once fully expanded (STU-044). */
  private lanes(route: Route, progress: number, prefix: string, stroke: StrokeStyle, z: number, out: Out[]): void {
    for (const [i, lane] of laneGeometry(route, progress).entries()) {
      lane.segments.forEach((c, k) => {
        out.push({ item: link(`${prefix}l${i}s${k}`, c, stroke, z, false), link: lane.link });
      });
      if (this.lod.labels && progress >= 1) {
        out.push({
          item: {
            kind: "text",
            id: `${prefix}t${i}`,
            x: lane.label.x - LABEL_W / 2,
            y: lane.label.y - LABEL_SIZE * 1.6,
            w: LABEL_W,
            text: lane.link.title,
            size: LABEL_SIZE,
            color: this.style.labelText,
            z: z + 1,
          },
          link: lane.link,
        });
      }
    }
  }

  /** The count badge at the bundle's midpoint (STU-041). */
  private badge(route: Route, prefix: string, fill: ColorRgba, z: number, out: Out[]): void {
    const mid = routeMidpoint(route);
    const text = String(route.bundle.links.length);
    out.push({
      item: {
        kind: "rect",
        id: `${prefix}badge`,
        x: mid.x - BADGE_W / 2,
        y: mid.y - BADGE_H / 2,
        w: BADGE_W,
        h: BADGE_H,
        radius: BADGE_H / 2,
        fill,
        z: z + 1,
      },
      link: undefined,
    });
    out.push({
      item: {
        kind: "text",
        id: `${prefix}count`,
        x: mid.x - text.length * 3.4,
        y: mid.y - BADGE_H / 2 + 1,
        w: BADGE_W,
        text,
        size: LABEL_SIZE,
        color: this.style.badgeText,
        z: z + 2,
      },
      link: undefined,
    });
  }
}

/** The arrowhead: two strokes back from the target anchor, around the side normal. */
function head(route: Route, prefix: string, stroke: StrokeStyle, z: number, out: Out[]): void {
  const tip = (route.segments.at(-1) as Cubic).to;
  const n = sideNormal(route.targetSide);
  for (const [k, angle] of [HEAD_ANGLE, -HEAD_ANGLE].entries()) {
    const cos = Math.cos(angle);
    const sin = Math.sin(angle);
    const end = { x: tip.x + HEAD * (n.x * cos - n.y * sin), y: tip.y + HEAD * (n.x * sin + n.y * cos) };
    out.push({ item: segment(`${prefix}h${k}`, tip, end, stroke, z), link: undefined });
  }
}
