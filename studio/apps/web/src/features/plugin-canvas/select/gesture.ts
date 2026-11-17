// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Guides } from "@plux/canvas";
import { uuidv7 } from "../screens.ts";
import { type DropPlan, planDrop, wrapNode } from "./drop-rules.ts";
import {
  duplicateRecipe,
  type EditFn,
  moveRecipe,
  moveToRecipe,
  putRecipe,
  reparentRecipe,
  resizeRecipe,
  stackDropRecipe,
  takeOutRecipe,
  unwrapRecipe,
  wrapRecipe,
} from "./edits.ts";
import {
  type Box,
  type DistanceLabel,
  distances,
  type Handle,
  resizeBox,
  type SnapResult,
  snapBox,
  translate,
} from "./geometry.ts";
import type { DropTarget } from "./reparent.ts";
import type { NodeIndex } from "./tree.ts";

export type GestureKind = "move" | "resize";

export interface MovePlan {
  /** The snapped offset of the dragged box. */
  readonly delta: { readonly x: number; readonly y: number };
  readonly guides: Guides;
  /** The snaps to hold on the next move (hysteresis). */
  readonly locks: SnapResult["locks"];
}

/**
 * The distances worth showing: to the nearest box on each side (left, right, above, below), not to every sibling, so
 * a crowded parent shows four labels at most instead of a cloud of numbers.
 */
function nearestDistances(box: Box, others: readonly Box[]): DistanceLabel[] {
  const best = new Map<string, DistanceLabel>();
  for (const o of others) {
    for (const d of distances(box, o)) {
      const before = d.axis === "x" ? d.to <= box.l + 1e-6 : d.to <= box.t + 1e-6;
      const side = `${d.axis}:${before ? "before" : "after"}`;
      const was = best.get(side);
      if (was === undefined || d.value < was.value) {
        best.set(side, d);
      }
    }
  }
  return [...best.values()];
}

/** Smart guides and distance labels for dragging \`box\` by \`delta\` among \`others\`. */
export function planMove(
  box: Box,
  others: readonly Box[],
  delta: { x: number; y: number },
  zoom: number,
  held: SnapResult["locks"] = {},
): MovePlan {
  const moved = translate(box, delta);
  const snap = snapBox(moved, others, zoom, held);
  const final = translate(moved, snap.delta);
  const labels = nearestDistances(final, others).map((d) => ({
    x: d.axis === "x" ? (d.from + d.to) / 2 : d.across,
    y: d.axis === "y" ? (d.from + d.to) / 2 : d.across,
    text: String(Math.round(d.value)),
  }));
  return {
    delta: { x: delta.x + snap.delta.x, y: delta.y + snap.delta.y },
    guides: { lines: snap.guides, labels },
    locks: snap.locks,
  };
}

export interface GestureEnv {
  readonly index: NodeIndex;
  readonly edit: EditFn;
  readonly docOf: (id: string) => string | undefined;
  readonly boxOf: (id: string) => Box | undefined;
  readonly typeOf: (id: string) => string | undefined;
  /** A resized width or height snapped to a dimension token near it (PC-18); a whole pixel when absent. */
  readonly snapSize?: (value: number) => number;
  /** Layers that neither move nor take another widget in. */
  readonly locked?: (id: string) => boolean;
  /** Whether the canvas previews a right-to-left locale. */
  readonly rtl?: boolean;
}

/**
 * The Stack that places `id` through a `Positioned` (the node is the `Positioned`, or its child), or undefined. Such a
 * node moves by its offset inside that Stack wherever the pointer goes: a `Positioned` anywhere else is an error on the
 * device (Flutter's parent-data rule), and a Stack's box can be smaller than its positioned children.
 */
export function placingStack(env: GestureEnv, id: string): string | undefined {
  const parent = env.index.parent.get(id) ?? undefined;
  const positioned =
    env.typeOf(id) === "Positioned"
      ? id
      : parent !== undefined && env.typeOf(parent) === "Positioned"
        ? parent
        : undefined;
  const stack = positioned === undefined ? undefined : (env.index.parent.get(positioned) ?? undefined);
  return stack !== undefined && env.typeOf(stack) === "Stack" ? stack : undefined;
}

/**
 * Where dragging `ids` to world point (x, y) drops them, by the drop rules (PC-20): into the innermost widget under the
 * pointer that can take them, on any screen, or refused with the reason. A node a `Positioned` places in a Stack moves
 * by its offset in that Stack wherever the pointer goes.
 */
export function moveDrop(env: GestureEnv, ids: readonly string[], x: number, y: number): DropPlan {
  const first = ids[0];
  const stack = first === undefined ? undefined : placingStack(env, first);
  const box = stack === undefined ? undefined : env.boxOf(stack);
  if (stack !== undefined && box !== undefined) {
    const [px, py] = [Math.round(x), Math.round(y)];
    return {
      ok: true,
      target: {
        parent: stack,
        index: env.index.children.get(stack)?.length ?? 0,
        indicator: { l: px, t: py, r: px, b: py },
        position: { left: px - Math.round(box.l), top: py - Math.round(box.t) },
      },
    };
  }
  return planDrop(env, { type: (first === undefined ? undefined : env.typeOf(first)) ?? "", ids }, x, y);
}

/** Where dragging `ids` to world point (x, y) drops them, or null when the drop is refused. */
export function dropAt(env: GestureEnv, ids: readonly string[], x: number, y: number): DropTarget | null {
  const plan = moveDrop(env, ids, x, y);
  return plan.ok ? plan.target : null;
}

/** The insertion indicator while dragging `ids` over the screens at world point (x, y). */
export function reparentPreview(env: GestureEnv, ids: readonly string[], x: number, y: number): Guides | null {
  const drop = dropAt(env, ids, x, y);
  return drop === null ? null : { lines: [], labels: [], indicator: drop.indicator };
}

/** The wrapper a drop puts the moved node in, with a fresh id, when it needs one. */
const wrapperOf = (drop: DropTarget, fresh: () => string) =>
  drop.wrap === undefined ? undefined : (node: unknown) => wrapNode(drop.wrap as string, fresh(), node);

/** Whether dropping `id` at `drop` leaves it where it is. */
function staysPut(env: GestureEnv, id: string, drop: DropTarget): boolean {
  const slot = env.index.slot.get(id) ?? undefined;
  if (env.index.parent.get(id) !== drop.parent || slot !== drop.slot || drop.wrap !== undefined) {
    return false;
  }
  return drop.slot !== undefined && drop.list !== true ? true : !movesAmongSiblings(env, id, drop);
}

/**
 * Ends a drag: a Stack child moves by the offset; anything else goes where the drop rules put it, on its own screen or
 * another. Returns the plan, refused when the drop was not written.
 */
export function finishMove(
  env: GestureEnv,
  id: string,
  delta: { x: number; y: number },
  x: number,
  y: number,
  fresh: () => string = uuidv7,
): DropPlan | null {
  const doc = env.docOf(id);
  if (doc === undefined) {
    return null;
  }
  const plan = moveDrop(env, [id], x, y);
  if (!plan.ok) {
    return plan;
  }
  const drop = plan.target;
  const to = env.docOf(drop.parent);
  if (to !== undefined && to !== doc && drop.position === undefined) {
    moveAcross(env, id, doc, to, drop, fresh);
    return plan;
  }
  const parent = env.index.parent.get(id);
  if (drop.position !== undefined) {
    const sameStack = parent === drop.parent || (parent != null && env.index.parent.get(parent) === drop.parent);
    const recipe = sameStack
      ? moveRecipe(id, delta.x, delta.y)
      : stackDropRecipe(id, drop.parent, drop.position, fresh);
    env.edit(doc, recipe, { group: "move" });
    return plan;
  }
  if (staysPut(env, id, drop)) {
    return plan;
  }
  // The insertion index counts the list without the dragged node, as the recipes insert after taking it out.
  const recipe =
    drop.slot === undefined && drop.wrap === undefined
      ? reparentRecipe([id], drop.parent, drop.index)
      : moveToRecipe(id, drop, wrapperOf(drop, fresh));
  env.edit(doc, recipe, { group: "move" });
  return plan;
}

/**
 * A widget moved onto another screen: taken out of its page and put into the other at the drop, one undo step across
 * the two pages (PC-20). Its ids are kept: they are unique in the app.
 */
function moveAcross(
  env: GestureEnv,
  id: string,
  from: string,
  to: string,
  drop: DropTarget,
  fresh: () => string,
): void {
  const group = `move-across:${id}:${Date.now()}`;
  let taken: unknown;
  env.edit(
    from,
    takeOutRecipe(id, (node) => {
      taken = node;
    }),
    { group },
  );
  if (taken === undefined) {
    return;
  }
  const wrap = wrapperOf(drop, fresh);
  env.edit(to, putRecipe(wrap === undefined ? taken : wrap(taken), drop), { group });
}

/** Whether a drop into the dragged node's own list puts it at another place than where it is. */
function movesAmongSiblings(env: GestureEnv, id: string, drop: DropTarget): boolean {
  const slot = drop.slot ?? null;
  const items = (env.index.children.get(drop.parent) ?? []).filter((k) => (env.index.slot.get(k) ?? null) === slot);
  const at = items.indexOf(id);
  return at >= 0 && drop.index !== at && drop.index <= items.length - 1;
}

/** Ends a resize from `handle` (the south-east corner when absent). */
export function finishResize(
  env: GestureEnv,
  id: string,
  delta: { x: number; y: number },
  handle: Handle = "se",
): void {
  const doc = env.docOf(id);
  const box = env.boxOf(id);
  if (doc === undefined || box === undefined) {
    return;
  }
  const next = resizeBox(box, handle, delta);
  const snap = env.snapSize ?? Math.round;
  env.edit(doc, resizeRecipe(id, snap(next.r - next.l), snap(next.b - next.t)), { group: "resize" });
}

/** Wrap, unwrap and duplicate for the commands; IDs are fresh per call. */
export function structureEdit(
  env: GestureEnv,
  kind: "wrap" | "unwrap" | "duplicate",
  ids: readonly string[],
  fresh: () => string,
): void {
  for (const id of ids) {
    const doc = env.docOf(id);
    if (doc === undefined) {
      continue;
    }
    const recipe =
      kind === "wrap"
        ? wrapRecipe(id, fresh(), "Column")
        : kind === "unwrap"
          ? unwrapRecipe(id)
          : duplicateRecipe(id, () => fresh());
    env.edit(doc, recipe, { group: kind });
  }
}
