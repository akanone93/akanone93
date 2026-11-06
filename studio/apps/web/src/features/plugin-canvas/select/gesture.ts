// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Guides } from "@plux/canvas";
import { widget } from "@plux/schema";
import { uuidv7 } from "../screens.ts";
import {
  duplicateRecipe,
  type EditFn,
  intoSlotRecipe,
  moveRecipe,
  putAtRecipe,
  putInSlotRecipe,
  reparentRecipe,
  resizeRecipe,
  stackDropRecipe,
  takeOutRecipe,
  unwrapRecipe,
  wrapRecipe,
} from "./edits.ts";
import { type Box, distances, type Handle, resizeBox, snapBox, translate } from "./geometry.ts";
import { type Container, type DropTarget, resolveDrop } from "./reparent.ts";
import type { NodeIndex } from "./tree.ts";

export type GestureKind = "move" | "resize";

export interface MovePlan {
  /** The snapped offset of the dragged box. */
  readonly delta: { readonly x: number; readonly y: number };
  readonly guides: Guides;
}

/** Smart guides and distance labels for dragging \`box\` by \`delta\` among \`others\`. */
export function planMove(box: Box, others: readonly Box[], delta: { x: number; y: number }, zoom: number): MovePlan {
  const moved = translate(box, delta);
  const snap = snapBox(moved, others, zoom);
  const final = translate(moved, snap.delta);
  const labels = others
    .flatMap((o) => distances(final, o))
    .map((d) => ({
      x: d.axis === "x" ? (d.from + d.to) / 2 : d.across,
      y: d.axis === "y" ? (d.from + d.to) / 2 : d.across,
      text: String(Math.round(d.value)),
    }));
  return {
    delta: { x: delta.x + snap.delta.x, y: delta.y + snap.delta.y },
    guides: { lines: snap.guides, labels },
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
}

const FLOW_TYPES = new Set(["Row", "Column", "Wrap", "Flex", "Stack"]);

/** The slot an empty single-child holder (a `Padding`, a `Center`, a `Card`, a `SizedBox`…) takes a drop in (PC-20). */
function emptySlotOf(type: string, kids: readonly string[]): string | undefined {
  const d = widget(type);
  if (d === undefined || d.children !== undefined || kids.length > 0) {
    return undefined;
  }
  return d.slots.some((s) => s.name === "child") ? "child" : undefined;
}

/** The flow containers that take a drop, with their children's boxes; only those of screen `doc` when given. */
export function containersOf(env: GestureEnv, doc?: string): Container[] {
  const out: Container[] = [];
  for (const [id, kids] of env.index.children) {
    const c = containerOf(env, id, kids, doc);
    if (c !== undefined) {
      out.push(c);
    }
  }
  return out;
}

/** Node `id` as a container that takes a drop: a flow container, or an empty single-child holder; else undefined. */
function containerOf(
  env: GestureEnv,
  id: string,
  kids: readonly string[],
  doc: string | undefined,
): Container | undefined {
  const type = env.typeOf(id);
  const box = env.boxOf(id);
  if (box === undefined || type === undefined || (doc !== undefined && env.docOf(id) !== doc)) {
    return undefined;
  }
  const flow = FLOW_TYPES.has(type);
  const slot = flow ? undefined : emptySlotOf(type, kids);
  if (!flow && slot === undefined) {
    return undefined;
  }
  const children = kids.flatMap((k) => {
    const b = env.boxOf(k);
    return b === undefined ? [] : [{ id: k, box: b }];
  });
  if (slot !== undefined) {
    return { id, box, axis: "slot", children, slot };
  }
  return { id, box, axis: type === "Row" ? "row" : type === "Stack" ? "stack" : "column", children };
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

/** Where dragging \`ids\` to world point (x, y) drops them: a flow container of their own screen, or null. */
export function dropAt(env: GestureEnv, ids: readonly string[], x: number, y: number): DropTarget | null {
  const first = ids[0];
  const stack = first === undefined ? undefined : placingStack(env, first);
  const box = stack === undefined ? undefined : env.boxOf(stack);
  if (stack !== undefined && box !== undefined) {
    const [px, py] = [Math.round(x), Math.round(y)];
    return {
      parent: stack,
      index: env.index.children.get(stack)?.length ?? 0,
      indicator: { l: px, t: py, r: px, b: py },
      position: { left: px - Math.round(box.l), top: py - Math.round(box.t) },
    };
  }
  // Any screen's containers take a drop: a widget moves onto another screen as well (PC-20).
  return resolveDrop(env.index, containersOf(env), ids, x, y);
}

/** The insertion indicator while dragging \`ids\` over flow containers at world point (x, y). */
export function reparentPreview(env: GestureEnv, ids: readonly string[], x: number, y: number): Guides | null {
  const drop = dropAt(env, ids, x, y);
  return drop === null ? null : { lines: [], labels: [], indicator: drop.indicator };
}

/** Ends a drag: Stack children move by the offset, flow children are reparented at the indicator. */
export function finishMove(
  env: GestureEnv,
  id: string,
  delta: { x: number; y: number },
  x: number,
  y: number,
  fresh: () => string = uuidv7,
): void {
  const doc = env.docOf(id);
  if (doc === undefined) {
    return;
  }
  const drop = dropAt(env, [id], x, y);
  const to = drop === null ? undefined : env.docOf(drop.parent);
  if (drop !== null && to !== undefined && to !== doc && drop.position === undefined) {
    moveAcross(env, id, doc, to, drop);
    return;
  }
  if (drop?.slot !== undefined) {
    env.edit(doc, intoSlotRecipe(id, drop.parent, drop.slot), { group: "move" });
    return;
  }
  const parent = env.index.parent.get(id);
  const sameStack = parent === drop?.parent || (parent != null && env.index.parent.get(parent) === drop?.parent);
  if (drop?.position !== undefined && !sameStack) {
    env.edit(doc, stackDropRecipe(id, drop.parent, drop.position, fresh), { group: "move" });
    return;
  }
  // Into another flow container, or to another place among its siblings: the insertion indicator's index counts the
  // children without the dragged one, as the recipe inserts after taking it out.
  if (drop !== null && drop.position === undefined && (drop.parent !== parent || movesAmongSiblings(env, id, drop))) {
    env.edit(doc, reparentRecipe([id], drop.parent, drop.index), { group: "move" });
    return;
  }
  env.edit(doc, moveRecipe(id, delta.x, delta.y), { group: "move" });
}

/**
 * A widget moved onto another screen: taken out of its page and put into the other at the drop, one undo step across
 * the two pages (PC-20). Its ids are kept: they are unique in the app.
 */
function moveAcross(env: GestureEnv, id: string, from: string, to: string, drop: DropTarget): void {
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
  const node = taken;
  env.edit(
    to,
    drop.slot !== undefined
      ? putInSlotRecipe(node, drop.parent, drop.slot)
      : putAtRecipe(node, drop.parent, drop.index),
    {
      group,
    },
  );
}

/** Whether a drop into the dragged node's own list puts it at another place than where it is. */
function movesAmongSiblings(env: GestureEnv, id: string, drop: DropTarget): boolean {
  const kids = (env.index.children.get(drop.parent) ?? []).filter((k) => k !== id);
  const at = (env.index.children.get(drop.parent) ?? []).indexOf(id);
  return at >= 0 && drop.index !== at && drop.index <= kids.length;
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
