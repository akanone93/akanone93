// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Motion on the canvas (PC-29, ANI-001–ANI-004, ANI-007): a widget's animation — its implicit animation of changed
 * values, its enter and exit transitions, its hero tag — read and written; the curves of the `curve` enum as Flutter's
 * `Curves` define them; the frame of an enter or exit transition at a moment, drawn on the canvas by wrapping the
 * widget in an `Opacity` and a `Transform` (ids ending `~motion`, so a click still selects the widget); and the widgets
 * of other screens that share a hero tag with the selection.
 */

import { COPY_MARK } from "./list-preview.ts";
import { findNode } from "./select/edits.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

export const CURVES = [
  "linear",
  "easeIn",
  "easeOut",
  "easeInOut",
  "fastOutSlowIn",
  "decelerate",
  "bounceIn",
  "bounceOut",
  "elasticOut",
  "overshoot",
] as const;
export type CurveName = (typeof CURVES)[number];

export const TRANSITION_KINDS = ["fade", "scale", "slideUp", "slideDown", "slideLeft", "slideRight"] as const;
export type TransitionKind = (typeof TRANSITION_KINDS)[number];

/** The default duration of an enter or exit transition, as the runtime plays it. */
export const DEFAULT_MS = 300;

/** `Cubic(a, b, c, d).transform(t)`: the y of the Bézier at the x `t`, found by bisection as Flutter does. */
function cubic(a: number, b: number, c: number, d: number, t: number): number {
  const at = (p1: number, p2: number, m: number) =>
    3 * p1 * (1 - m) * (1 - m) * m + 3 * p2 * (1 - m) * m * m + m * m * m;
  let lo = 0;
  let hi = 1;
  for (let i = 0; i < 30; i += 1) {
    const mid = (lo + hi) / 2;
    if (at(a, c, mid) < t) {
      lo = mid;
    } else {
      hi = mid;
    }
  }
  return at(b, d, (lo + hi) / 2);
}

/** `_bounce` of Flutter's `BounceOutCurve`. */
function bounce(t: number): number {
  if (t < 1 / 2.75) {
    return 7.5625 * t * t;
  }
  if (t < 2 / 2.75) {
    const u = t - 1.5 / 2.75;
    return 7.5625 * u * u + 0.75;
  }
  if (t < 2.5 / 2.75) {
    const u = t - 2.25 / 2.75;
    return 7.5625 * u * u + 0.9375;
  }
  const u = t - 2.625 / 2.75;
  return 7.5625 * u * u + 0.984375;
}

/** A curve's value at `t` (0–1), as `Curves.<name>.transform(t)`. */
export function curveAt(curve: string | undefined, t: number): number {
  const x = Math.min(1, Math.max(0, t));
  switch (curve) {
    case "easeIn":
      return cubic(0.42, 0, 1, 1, x);
    case "easeOut":
      return cubic(0, 0, 0.58, 1, x);
    case "easeInOut":
      return cubic(0.42, 0, 0.58, 1, x);
    case "fastOutSlowIn":
      return cubic(0.4, 0, 0.2, 1, x);
    case "decelerate":
      return 1 - (1 - x) * (1 - x);
    case "bounceIn":
      return 1 - bounce(1 - x);
    case "bounceOut":
      return bounce(x);
    case "elasticOut":
      return x === 0 || x === 1 ? x : 2 ** (-10 * x) * Math.sin(((x - 0.4 / 4) * (Math.PI * 2)) / 0.4) + 1;
    case "overshoot":
      // `Curves.easeOutBack`.
      return cubic(0.175, 0.885, 0.32, 1.275, x);
    default:
      return x;
  }
}

/** What a transition draws at one moment: the widget's opacity, its offset in pixels and its scale. */
export interface MotionFrame {
  readonly opacity: number;
  readonly dx: number;
  readonly dy: number;
  readonly scale: number;
}

export const STILL: MotionFrame = { opacity: 1, dx: 0, dy: 0, scale: 1 };

/**
 * The frame of a transition of `kind` at progress `p` (0 hidden, 1 shown): a fade's opacity, a scale from nothing, a
 * slide over the widget's own size (`SlideTransition` offsets are fractions of the child's size).
 */
export function transitionFrame(
  kind: string,
  p: number,
  size: { readonly w: number; readonly h: number },
): MotionFrame {
  const away = 1 - p;
  switch (kind) {
    case "fade":
      return { ...STILL, opacity: p };
    case "scale":
      return { ...STILL, scale: p };
    case "slideUp":
      return { ...STILL, dy: away * size.h };
    case "slideDown":
      return { ...STILL, dy: -away * size.h };
    case "slideLeft":
      return { ...STILL, dx: away * size.w };
    case "slideRight":
      return { ...STILL, dx: -away * size.w };
    default:
      return STILL;
  }
}

/** The frame at `elapsed` ms of a node's `phase` transition; undefined once it has ended. */
export function frameAt(
  transition: Obj,
  phase: "enter" | "exit",
  elapsed: number,
  size: { readonly w: number; readonly h: number },
): MotionFrame | undefined {
  const duration = num(transition["durationMs"]) ?? DEFAULT_MS;
  if (elapsed > duration) {
    return undefined;
  }
  const eased = curveAt(typeof transition["curve"] === "string" ? transition["curve"] : undefined, elapsed / duration);
  return transitionFrame(String(transition["kind"]), phase === "enter" ? eased : 1 - eased, size);
}

/** A node's `animation`. */
export function animationOf(node: unknown): Obj {
  return isObj(node) && isObj(node["animation"]) ? node["animation"] : {};
}

/**
 * The recipe that sets one key of node `id`'s animation (`durationMs`, `curve`, `delayMs`, `enter`, `exit`, `hero`,
 * `reduceMotion`); undefined removes it, and an animation left empty is removed.
 */
export function setAnimationRecipe(id: string, key: string, value: unknown): (root: unknown) => void {
  return (root) => {
    const node = findNode(root, id)?.node as Obj | undefined;
    if (node === undefined) {
      return;
    }
    const animation: Obj = isObj(node["animation"]) ? node["animation"] : {};
    if (value === undefined || value === "") {
      delete animation[key];
    } else {
      animation[key] = value;
    }
    if (Object.keys(animation).length === 0) {
      delete node["animation"];
    } else {
      node["animation"] = animation;
    }
  };
}

/** `root` with node `id` drawn at `frame`: inside an `Opacity` and a `Transform` that only the canvas draws. */
export function withMotion(root: unknown, id: string, frame: MotionFrame): unknown {
  if (!isObj(root)) {
    return root;
  }
  const wrap = (node: Obj): Obj => ({
    id: `${id}${COPY_MARK}motion`,
    type: "Opacity",
    props: { opacity: Math.min(1, Math.max(0, frame.opacity)) },
    slots: {
      child: {
        id: `${id}${COPY_MARK}motion-move`,
        type: "Transform",
        props: { offset: { dx: frame.dx, dy: frame.dy }, scale: Math.max(0.0001, frame.scale), alignment: "center" },
        slots: { child: node },
      },
    },
  });
  const visit = (n: unknown): unknown => {
    if (Array.isArray(n)) {
      return n.map(visit);
    }
    if (!isObj(n)) {
      return n;
    }
    if (n["id"] === id) {
      return wrap(n);
    }
    const out: Obj = { ...n };
    if (Array.isArray(n["children"])) {
      out["children"] = (n["children"] as unknown[]).map(visit);
    }
    if (isObj(n["slots"])) {
      out["slots"] = Object.fromEntries(Object.entries(n["slots"]).map(([k, v]) => [k, visit(v)]));
    }
    return out;
  };
  return visit(root);
}

/** A node's literal hero tag, if it has one. */
export const heroTagOf = (node: unknown): string | undefined => {
  const hero = animationOf(node)["hero"];
  return typeof hero === "string" && hero !== "" ? hero : undefined;
};

/** The children and slot contents of a node. */
const kidsOf = (n: Obj): unknown[] => [
  ...(Array.isArray(n["children"]) ? n["children"] : []),
  ...(isObj(n["slots"]) ? Object.values(n["slots"]).flatMap((v) => (Array.isArray(v) ? v : [v])) : []),
];

/** Every widget of `roots` with a literal hero tag. */
function tagged(roots: readonly unknown[]): { readonly id: string; readonly tag: string }[] {
  const out: { id: string; tag: string }[] = [];
  const visit = (n: unknown) => {
    if (!isObj(n)) {
      return;
    }
    const t = heroTagOf(n);
    if (t !== undefined && typeof n["id"] === "string") {
      out.push({ id: n["id"], tag: t });
    }
    kidsOf(n).forEach(visit);
  };
  roots.forEach(visit);
  return out;
}

/** The widgets of `roots` that share a hero tag with node `id` (ANI-004), other than itself. */
export function heroMatches(roots: readonly unknown[], id: string): string[] {
  const all = tagged(roots);
  const tag = all.find((t) => t.id === id)?.tag;
  return tag === undefined ? [] : all.filter((t) => t.tag === tag && t.id !== id).map((t) => t.id);
}
