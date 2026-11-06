// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Padding and gap handles (PC-18, STU-067): what dragging a widget's padding or its children's gap writes. A value
 * snaps to a theme's spacing token near it and is written as a reference to it (`{"$token": "space.md"}`), else to a
 * whole pixel; padding keeps the shape the document gave it where it can (`all`, `vertical`/`horizontal`, sides,
 * `start`/`end`).
 */

import { findNode } from "./select/edits.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

export type Side = "top" | "right" | "bottom" | "left";
export const sides: readonly Side[] = ["top", "right", "bottom", "left"];

/** A spacing value: pixels, or a token's name. */
export type Spacing = number | { readonly $token: string };

/** The widget types whose `padding` prop the handles drag, and the flex types whose `spacing` they drag. */
const PADDING_TYPES: ReadonlySet<string> = new Set([
  "Padding",
  "AnimatedPadding",
  "Container",
  "AnimatedContainer",
  "ListView",
  "GridView",
  "SingleChildScrollView",
]);
const GAP_TYPES: ReadonlySet<string> = new Set(["Column", "Row", "Flex", "Wrap"]);

export const dragsPadding = (type: string | undefined): boolean => type !== undefined && PADDING_TYPES.has(type);
export const dragsGap = (type: string | undefined): boolean => type !== undefined && GAP_TYPES.has(type);

/** A spacing value in pixels: a number, or its token's value (0 for an unknown token). */
export function pixels(v: unknown, dims: Readonly<Record<string, number>>): number {
  if (typeof v === "number") {
    return v;
  }
  return isObj(v) && typeof v["$token"] === "string" ? (dims[v["$token"]] ?? 0) : 0;
}

/** A padding prop's four sides as written (numbers or token references), physical sides; `directional` when it uses start/end. */
export function readSides(
  value: unknown,
  rtl: boolean,
): { readonly sides: Record<Side, Spacing>; readonly directional: boolean } {
  const zero: Record<Side, Spacing> = { top: 0, right: 0, bottom: 0, left: 0 };
  if (Array.isArray(value)) {
    const [l, t, r, b] = value as Spacing[];
    return { sides: { top: t ?? 0, right: r ?? 0, bottom: b ?? 0, left: l ?? 0 }, directional: false };
  }
  if (!isObj(value)) {
    return { sides: zero, directional: false };
  }
  const pick = (...keys: string[]): Spacing => {
    for (const k of keys) {
      const v = value[k];
      if (typeof v === "number" || (isObj(v) && typeof v["$token"] === "string")) {
        return v as Spacing;
      }
    }
    return 0;
  };
  const directional = "start" in value || "end" in value;
  const start = pick("start", "horizontal", "all");
  const end = pick("end", "horizontal", "all");
  return {
    sides: {
      top: pick("top", "vertical", "all"),
      bottom: pick("bottom", "vertical", "all"),
      left: directional ? (rtl ? end : start) : pick("left", "horizontal", "all"),
      right: directional ? (rtl ? start : end) : pick("right", "horizontal", "all"),
    },
    directional,
  };
}

const same = (a: Spacing, b: Spacing): boolean => JSON.stringify(a) === JSON.stringify(b);

/** Four sides as a padding prop: `all`, `vertical`/`horizontal`, or each side, in the start/end form when asked. */
export function writeSides(s: Record<Side, Spacing>, directional: boolean, rtl: boolean): Obj {
  if (same(s.top, s.bottom) && same(s.left, s.right)) {
    return same(s.top, s.left) ? { all: s.top } : { vertical: s.top, horizontal: s.left };
  }
  if (directional) {
    return { top: s.top, bottom: s.bottom, start: rtl ? s.right : s.left, end: rtl ? s.left : s.right };
  }
  return { top: s.top, right: s.right, bottom: s.bottom, left: s.left };
}

/**
 * A dragged value: the nearest spacing token within `reach` pixels, as a reference to it, else a whole pixel; never
 * below 0. Ties go to the token named first.
 */
export function snapSpacing(value: number, dims: Readonly<Record<string, number>>, reach: number): Spacing {
  const v = Math.max(0, value);
  let best: { name: string; d: number } | undefined;
  for (const [name, px] of Object.entries(dims)) {
    const d = Math.abs(px - v);
    if (name.startsWith("space") && d <= reach && (best === undefined || d < best.d)) {
      best = { name, d };
    }
  }
  return best === undefined ? Math.round(v) : { $token: best.name };
}

/** A resized width or height: a dimension token near it, as its pixels, else a whole pixel (PC-18). */
export function snapSize(value: number, dims: Readonly<Record<string, number>>, reach: number): number {
  let best: { px: number; d: number } | undefined;
  for (const px of Object.values(dims)) {
    const d = Math.abs(px - value);
    if (d <= reach && (best === undefined || d < best.d)) {
      best = { px, d };
    }
  }
  return best?.px ?? Math.round(value);
}

/** The sides a drag of `side` changes: it alone, it and its opposite (Alt), or all four (Shift). */
export function draggedSides(side: Side, alt: boolean, shift: boolean): Side[] {
  if (shift) {
    return [...sides];
  }
  const opposite: Record<Side, Side> = { top: "bottom", bottom: "top", left: "right", right: "left" };
  return alt ? [side, opposite[side]] : [side];
}

/** The recipe that sets node `id`'s `padding` to `sides` (in the document's shape). */
export function paddingRecipe(
  id: string,
  next: Record<Side, Spacing>,
  directional: boolean,
  rtl: boolean,
): (root: unknown) => void {
  return (root) => {
    const at = findNode(root, id);
    if (at === null) {
      return;
    }
    const node = at.node as Obj;
    const props = isObj(node["props"]) ? (node["props"] as Obj) : {};
    node["props"] = { ...props, padding: writeSides(next, directional, rtl) };
  };
}

/** The recipe that sets node `id`'s `spacing` (the gap between its children). */
export function gapRecipe(id: string, value: Spacing): (root: unknown) => void {
  return (root) => {
    const at = findNode(root, id);
    if (at === null) {
      return;
    }
    const node = at.node as Obj;
    const props = isObj(node["props"]) ? (node["props"] as Obj) : {};
    node["props"] = { ...props, spacing: value };
  };
}
