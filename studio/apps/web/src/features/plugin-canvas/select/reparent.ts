// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Box } from "./geometry.ts";
import { lineage, type NodeIndex } from "./tree.ts";

/** A container that accepts children, with its direction. */
export interface Container {
  readonly id: string;
  readonly box: Box;
  readonly axis: "row" | "column" | "stack";
  /** Child boxes in order. */
  readonly children: readonly { readonly id: string; readonly box: Box }[];
}

export interface DropTarget {
  readonly parent: string;
  readonly index: number;
  /** The insertion indicator: a line in canvas coordinates. */
  readonly indicator: Box;
}

const inside = (b: Box, x: number, y: number): boolean => x >= b.l && x <= b.r && y >= b.t && y <= b.b;
const area = (b: Box): number => (b.r - b.l) * (b.b - b.t);

/**
 * Resolves the drop of `dragged` at (x, y): the smallest container under the pointer that is not the dragged
 * node or one of its descendants, and the insertion index among its children by the pointer's position along the axis.
 */
export function resolveDrop(
  index: NodeIndex,
  containers: readonly Container[],
  dragged: readonly string[],
  x: number,
  y: number,
): DropTarget | null {
  const blocked = (id: string): boolean => lineage(index, id).some((a) => dragged.includes(a));
  const candidates = containers.filter((c) => !blocked(c.id) && inside(c.box, x, y));
  candidates.sort((a, b) => area(a.box) - area(b.box));
  const c = candidates[0];
  if (c === undefined) {
    return null;
  }
  const kids = c.children.filter((k) => !dragged.includes(k.id));
  const pos = c.axis === "row" ? x : y;
  let at = kids.length;
  for (const [i, k] of kids.entries()) {
    const mid = c.axis === "row" ? (k.box.l + k.box.r) / 2 : (k.box.t + k.box.b) / 2;
    if (pos < mid) {
      at = i;
      break;
    }
  }
  const edge = (() => {
    const before = kids[at - 1];
    const after = kids[at];
    if (c.axis === "row") {
      const px = after !== undefined ? after.box.l : before !== undefined ? before.box.r : c.box.l;
      return { l: px, t: c.box.t, r: px, b: c.box.b };
    }
    const py = after !== undefined ? after.box.t : before !== undefined ? before.box.b : c.box.t;
    return { l: c.box.l, t: py, r: c.box.r, b: py };
  })();
  return { parent: c.id, index: at, indicator: edge };
}
