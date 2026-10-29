// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Bundle expansion into titled lanes (STU-044), the lane menu (STU-046) and
 * the arrows' level of detail (STU-048). Pure: the expansion is a function of
 * the route and a progress value the canvas frame loop advances with the
 * clock it is given, so geometry interpolates frame by frame and tests run
 * without timers.
 */

import type { Cubic, NavigateEdge, Route } from "./arrows.ts";
import { STRAIGHT_LINKS_BELOW } from "./lod.ts";
import type { Point } from "./protocol.ts";

/** Expansion and collapse take this long (STU-044: 150–200 ms). */
export const EXPAND_MS = 180;
/** Distance between neighbouring lanes, room for one title line. */
export const LANE_SPACING = 28;
/** Below this zoom arrows are straight, labels hidden and lanes merged (STU-048). */
export const ARROW_DETAIL_BELOW = STRAIGHT_LINKS_BELOW;

/** What arrows draw at a zoom level. */
export interface ArrowLod {
  /** One straight segment per bundle instead of routed curves. */
  readonly straight: boolean;
  /** Count badges and lane titles. */
  readonly labels: boolean;
  /** Expansion into lanes; merged into one arrow when false. */
  readonly lanes: boolean;
}

const detailed: ArrowLod = Object.freeze({ straight: false, labels: true, lanes: true });
const simple: ArrowLod = Object.freeze({ straight: true, labels: false, lanes: false });

/** The arrows' level of detail for `zoom` (STU-048). */
export function arrowLodForZoom(zoom: number): ArrowLod {
  return zoom < ARROW_DETAIL_BELOW ? simple : detailed;
}

/** One lane of an expanded bundle: one link (STU-044, STU-046). */
export interface Lane {
  readonly link: NavigateEdge;
  readonly segments: readonly Cubic[];
  /** Where the lane crosses the bundle's middle: its title sits here. */
  readonly label: Point;
}

/** Ease-out cubic on [0, 1]. */
export function easeOut(t: number): number {
  const s = 1 - Math.min(1, Math.max(0, t));
  return 1 - s * s * s;
}

function lerp(a: Point, b: Point, t: number): Point {
  return { x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t };
}

/** Splits `c` at `t` (de Casteljau). */
export function splitCubic(c: Cubic, t: number): [Cubic, Cubic] {
  const ab = lerp(c.from, c.c1, t);
  const bc = lerp(c.c1, c.c2, t);
  const cd = lerp(c.c2, c.to, t);
  const abc = lerp(ab, bc, t);
  const bcd = lerp(bc, cd, t);
  const m = lerp(abc, bcd, t);
  return [
    { from: c.from, c1: ab, c2: abc, to: m },
    { from: m, c1: bcd, c2: cd, to: c.to },
  ];
}

function shift(p: Point, n: Point, o: number): Point {
  return { x: p.x + n.x * o, y: p.y + n.y * o };
}

/**
 * The lanes of `route` at expansion `progress` (0 collapsed, 1 expanded). The
 * middle piece is split at its midpoint; each lane moves that point and its
 * two neighbouring control points sideways, so all lanes share the entry stem
 * at the source and merge back into one stem at the target, and at progress 0
 * every lane is exactly the collapsed curve.
 */
export function laneGeometry(route: Route, progress: number, spacing = LANE_SPACING): Lane[] {
  const pieces = route.segments;
  const index = Math.floor((pieces.length - 1) / 2);
  const middle = pieces[index] as Cubic;
  const [left, right] = splitCubic(middle, 0.5);
  const tx = right.c1.x - left.c2.x;
  const ty = right.c1.y - left.c2.y;
  const len = Math.hypot(tx, ty);
  const normal = len > 0 ? { x: -ty / len, y: tx / len } : { x: 0, y: 1 };
  const links = route.bundle.links;
  const n = links.length;
  const before = pieces.slice(0, index);
  const after = pieces.slice(index + 1);
  return links.map((link, i) => {
    const o = (i - (n - 1) / 2) * spacing * progress;
    const mid = shift(left.to, normal, o);
    const segments = [
      ...before,
      { from: left.from, c1: left.c1, c2: shift(left.c2, normal, o), to: mid },
      { from: mid, c1: shift(right.c1, normal, o), c2: right.c2, to: right.to },
      ...after,
    ];
    return { link, segments, label: mid };
  });
}

/** The point halfway along a route's middle piece: where the count badge sits (STU-041). */
export function routeMidpoint(route: Route): Point {
  const middle = route.segments[Math.floor((route.segments.length - 1) / 2)] as Cubic;
  return splitCubic(middle, 0.5)[0].to;
}

interface Tween {
  from: number;
  to: number;
  start: number;
}

/**
 * Expansion state of the bundles, advanced by the canvas frame loop: `focus`
 * on hover or keyboard focus, `progress` each frame. Collapsing runs the same
 * animation backwards from wherever the expansion was.
 */
export class Expansion {
  private readonly tweens = new Map<string, Tween>();
  private focused: string | null = null;

  /** The focused bundle, if any. */
  get focus(): string | null {
    return this.focused;
  }

  /** Focuses `id` (or nothing) at time `now` in ms; returns the ids whose animation changed. */
  setFocus(id: string | null, now: number): string[] {
    if (id === this.focused) {
      return [];
    }
    const changed: string[] = [];
    if (this.focused !== null) {
      this.tweens.set(this.focused, { from: this.linear(this.focused, now), to: 0, start: now });
      changed.push(this.focused);
    }
    if (id !== null) {
      this.tweens.set(id, { from: this.linear(id, now), to: 1, start: now });
      changed.push(id);
    }
    this.focused = id;
    return changed;
  }

  /** Eased expansion of `id` at `now`, in [0, 1]. */
  progress(id: string, now: number): number {
    return easeOut(this.linear(id, now));
  }

  /** The bundles still animating at `now`, sorted; finished collapses are forgotten. */
  animating(now: number): string[] {
    const out: string[] = [];
    for (const [id, tween] of this.tweens) {
      const value = this.linear(id, now);
      if (value !== tween.to) {
        out.push(id);
      }
    }
    return out.sort();
  }

  /** Drops finished collapses; returns whether any bundle is expanded or animating. */
  settle(now: number): boolean {
    for (const [id, tween] of [...this.tweens]) {
      if (tween.to === 0 && this.linear(id, now) === 0) {
        this.tweens.delete(id);
      }
    }
    return this.tweens.size > 0;
  }

  private linear(id: string, now: number): number {
    const tween = this.tweens.get(id);
    if (!tween) {
      return 0;
    }
    const step = Math.max(0, now - tween.start) / EXPAND_MS;
    return tween.to > tween.from ? Math.min(tween.to, tween.from + step) : Math.max(tween.to, tween.from - step);
  }
}

/** A lane menu action (STU-046). */
export type LaneAction = "open-source" | "open-target" | "change-trigger" | "redirect" | "remove";

/** One entry of the lane menu; the app supplies labels (i18n) and handlers. */
export interface LaneMenuItem {
  readonly action: LaneAction;
  readonly link: NavigateEdge;
}

const LANE_ACTIONS: readonly LaneAction[] = ["open-source", "open-target", "change-trigger", "redirect", "remove"];

/** The menu offered when a lane is clicked (STU-046), in display order. */
export function laneMenu(link: NavigateEdge): LaneMenuItem[] {
  return LANE_ACTIONS.map((action) => ({ action, link }));
}
