// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Canvas pixels within which two corner badge clusters would cover each other. */
export const CORNER_GAP = { x: 18, y: 14 } as const;

/** Canvas pixels within which a hovered widget's name tag would cover the selected widget's. */
export const TAG_GAP = { x: 120, y: 22 } as const;

/** A widget's badge cluster: where its top-end corner is on the canvas, and its size there. */
export interface Corner {
  readonly id: string;
  readonly x: number;
  readonly y: number;
  /** The widget's area in canvas pixels: the smaller of two widgets on one corner keeps it. */
  readonly area: number;
}

/**
 * The widgets whose badge clusters are drawn (C4): nested widgets often share a top-end corner (a Column and its first
 * Text), and their badges then covered each other. One cluster shows per corner: the hovered or selected widget's,
 * else the innermost widget's (the smallest); the others show theirs when hovered or selected.
 */
export function cornersShown(corners: readonly Corner[], focus: ReadonlySet<string>): ReadonlySet<string> {
  const order = [...corners].sort(
    (a, b) => Number(focus.has(b.id)) - Number(focus.has(a.id)) || a.area - b.area || (a.id < b.id ? -1 : 1),
  );
  const kept: Corner[] = [];
  for (const c of order) {
    const covered = kept.some((k) => Math.abs(k.x - c.x) < CORNER_GAP.x && Math.abs(k.y - c.y) < CORNER_GAP.y);
    if (!covered || focus.has(c.id)) {
      kept.push(c);
    }
  }
  return new Set(kept.map((c) => c.id));
}

/** Whether a hovered widget's name tag at `hover` stays clear of the selected widget's at `selected`. */
export function tagClear(
  hover: { readonly x: number; readonly y: number },
  selected: { readonly x: number; readonly y: number } | undefined,
): boolean {
  return (
    selected === undefined || Math.abs(hover.x - selected.x) >= TAG_GAP.x || Math.abs(hover.y - selected.y) >= TAG_GAP.y
  );
}
