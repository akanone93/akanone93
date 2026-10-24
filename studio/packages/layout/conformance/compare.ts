// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Compares Studio's layout with a Flutter golden within ADR-0013's
 * conformance tolerance (STU-005).
 */

/** `[x, y, width, height]` in logical pixels, as goldens store it. */
export type GoldenRect = [number, number, number, number];

/** ADR-0013: every edge of every node within half a logical pixel of Flutter's. */
export const edgeTolerance = 0.5;

/** ADR-0013: a size derived from text within one logical pixel of Flutter's. */
export const textTolerance = 1;

/** One node whose rect differs from the golden, or exists on one side only. */
export interface RectDifference {
  id: string;
  expected: GoldenRect | null;
  actual: GoldenRect | null;
  /** The largest edge deviation in logical pixels; infinite when a side is missing. */
  delta: number;
}

function edgeDelta(a: GoldenRect, b: GoldenRect): number {
  const edges = (r: GoldenRect): number[] => [r[0], r[1], r[0] + r[2], r[1] + r[3]];
  const ea = edges(a);
  const eb = edges(b);
  let max = 0;
  for (let i = 0; i < 4; i++) {
    const d = Math.abs((ea[i] ?? 0) - (eb[i] ?? 0));
    max = Number.isNaN(d) ? Number.POSITIVE_INFINITY : Math.max(max, d);
  }
  return max;
}

/** Every id whose rect deviates by more than `tolerance` on any edge, sorted by id. */
export function compareRects(
  actual: Record<string, GoldenRect>,
  expected: Record<string, GoldenRect>,
  tolerance: number | ((id: string) => number) = edgeTolerance,
): RectDifference[] {
  const toleranceFor = typeof tolerance === "number" ? () => tolerance : tolerance;
  const ids = [...new Set([...Object.keys(actual), ...Object.keys(expected)])].sort();
  const differences: RectDifference[] = [];
  for (const id of ids) {
    const a = actual[id] ?? null;
    const e = expected[id] ?? null;
    const delta = a === null || e === null ? Number.POSITIVE_INFINITY : edgeDelta(a, e);
    if (delta > toleranceFor(id)) {
      differences.push({ id, expected: e, actual: a, delta });
    }
  }
  return differences;
}
