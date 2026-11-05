// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Box, Handle, Point } from "./geometry.ts";

/** The eight resize handles of a box, in drawing order. */
export const HANDLES: readonly Handle[] = ["nw", "n", "ne", "e", "se", "s", "sw", "w"];

/** Where `handle` sits on `box`: a corner or the middle of a side. */
export function handlePoint(box: Box, handle: Handle): Point {
  const x = handle.includes("w") ? box.l : handle.includes("e") ? box.r : (box.l + box.r) / 2;
  const y = handle.includes("n") ? box.t : handle.includes("s") ? box.b : (box.t + box.b) / 2;
  return { x, y };
}

/**
 * The handle of `box` under `p` within `zone` (both in the same units), corners first, or undefined. A box too small
 * for its side handles keeps only the corners, so a press inside it still moves it.
 */
export function handleAt(box: Box, p: Point, zone: number): Handle | undefined {
  const small = box.r - box.l < zone * 4 || box.b - box.t < zone * 4;
  const order: readonly Handle[] = ["nw", "ne", "se", "sw", ...(small ? [] : (["n", "e", "s", "w"] as const))];
  return order.find((h) => {
    const at = handlePoint(box, h);
    return Math.abs(p.x - at.x) <= zone && Math.abs(p.y - at.y) <= zone;
  });
}

/** The CSS cursor of a handle, for the pointer over it. */
export function handleCursor(handle: Handle): string {
  return handle === "n" || handle === "s"
    ? "ns-resize"
    : handle === "e" || handle === "w"
      ? "ew-resize"
      : handle === "nw" || handle === "se"
        ? "nwse-resize"
        : "nesw-resize";
}
