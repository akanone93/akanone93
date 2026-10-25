// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Guides } from "@plux/canvas";
import { uuidv7 } from "../screens.ts";
import {
  duplicateRecipe,
  type EditFn,
  moveRecipe,
  reparentRecipe,
  resizeRecipe,
  stackDropRecipe,
  unwrapRecipe,
  wrapRecipe,
} from "./edits.ts";
import { type Box, distances, type Handle, resizeBox, snapBox, translate } from "./geometry.ts";
import { type Container, resolveDrop } from "./reparent.ts";
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
}

const FLOW_TYPES = new Set(["Row", "Column", "Wrap", "Flex", "Stack"]);

export function containersOf(env: GestureEnv): Container[] {
  const out: Container[] = [];
  for (const [id, kids] of env.index.children) {
    const type = env.typeOf(id);
    const box = env.boxOf(id);
    if (box === undefined || type === undefined || !FLOW_TYPES.has(type)) {
      continue;
    }
    const children = kids.flatMap((k) => {
      const b = env.boxOf(k);
      return b === undefined ? [] : [{ id: k, box: b }];
    });
    out.push({ id, box, axis: type === "Row" ? "row" : type === "Stack" ? "stack" : "column", children });
  }
  return out;
}

/** The insertion indicator while dragging \`ids\` over flow containers at world point (x, y). */
export function reparentPreview(env: GestureEnv, ids: readonly string[], x: number, y: number): Guides | null {
  const drop = resolveDrop(env.index, containersOf(env), ids, x, y);
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
  const drop = resolveDrop(env.index, containersOf(env), [id], x, y);
  const parent = env.index.parent.get(id);
  const sameStack = parent === drop?.parent || (parent != null && env.index.parent.get(parent) === drop?.parent);
  if (drop?.position !== undefined && !sameStack) {
    env.edit(doc, stackDropRecipe(id, drop.parent, drop.position, fresh), { group: "move" });
    return;
  }
  if (drop !== null && drop.position === undefined && drop.parent !== parent) {
    env.edit(doc, reparentRecipe([id], drop.parent, drop.index), { group: "move" });
    return;
  }
  env.edit(doc, moveRecipe(id, delta.x, delta.y), { group: "move" });
}

/** Ends a resize from the south-east handle. */
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
  env.edit(doc, resizeRecipe(id, Math.round(next.r - next.l), Math.round(next.b - next.t)), { group: "resize" });
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
