// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Where the Accessibility view's labels stand on screen (PC-31, #219): each label as a rectangle in canvas pixels, and
 * the ones kept so that none covers another — the findings first (low contrast, then small targets), then the focus
 * order, as the caller lists them. A label left out keeps its target's outline, and zooming in brings it back. Below
 * `A11Y_TEXT_ZOOM` the view keeps only the outlines: numbers and words there would cover the screens.
 */

/** A rectangle in canvas pixels. */
export interface Rect {
  readonly l: number;
  readonly t: number;
  readonly r: number;
  readonly b: number;
}

/** A label to place: its rectangle on screen and what it shows. */
export interface Label<T> {
  readonly rect: Rect;
  readonly mark: T;
}

/** The zoom below which the view draws no numbers or tags, only the small targets' outlines. */
export const A11Y_TEXT_ZOOM = 0.3;
/** A focus-order number's side, in CSS pixels (`.pc-a11y-order`). */
const BADGE = 18;
/** A tag's line, its width per character of 10-pixel text and its padding, in CSS pixels (`.pc-a11y-tag`). */
const TAG_LINE = 18;
const TAG_CHAR = 6;
const TAG_PAD = 12;
/** How far a small target's tag stands below the target, and a contrast tag above the text, in CSS pixels. */
const TAG_BELOW = 4;
const TAG_ABOVE = 18;
/** The side of a cell of the grid that finds a label's neighbours, in CSS pixels. */
const CELL = 64;

/** A focus-order number centred on the widget's top start. */
export const badgeAt = (x: number, y: number): Rect => ({
  l: x - BADGE / 2,
  t: y - BADGE / 2,
  r: x + BADGE / 2,
  b: y + BADGE / 2,
});

const widthOf = (text: string): number => text.length * TAG_CHAR + TAG_PAD;

/** A small target's tag under the target's bottom start. */
export const tagBelow = (x: number, y: number, text: string): Rect => ({
  l: x,
  t: y + TAG_BELOW,
  r: x + widthOf(text),
  b: y + TAG_BELOW + TAG_LINE,
});

/** A contrast tag ending at the text's top end, above it. */
export const tagAbove = (x: number, y: number, text: string): Rect => ({
  l: x - widthOf(text),
  t: y - TAG_ABOVE,
  r: x,
  b: y - TAG_ABOVE + TAG_LINE,
});

const overlaps = (a: Rect, b: Rect): boolean => a.l < b.r && b.l < a.r && a.t < b.b && b.t < a.b;

function cellsOf(r: Rect): string[] {
  const out: string[] = [];
  for (let i = Math.floor(r.l / CELL); i <= Math.floor(r.r / CELL); i++) {
    for (let j = Math.floor(r.t / CELL); j <= Math.floor(r.b / CELL); j++) {
      out.push(`${i}:${j}`);
    }
  }
  return out;
}

/**
 * The labels kept, in the order given: each that stands in the view and overlaps none kept before it. A grid of cells
 * finds the labels near each one, so a view of thousands of widgets places them in linear time.
 */
export function keepApart<T>(
  labels: readonly Label<T>[],
  view: { readonly width: number; readonly height: number },
): Label<T>[] {
  const grid = new Map<string, Rect[]>();
  const kept: Label<T>[] = [];
  for (const label of labels) {
    const r = label.rect;
    if (r.r < 0 || r.b < 0 || r.l > view.width || r.t > view.height) {
      continue;
    }
    const cells = cellsOf(r);
    if (cells.some((c) => grid.get(c)?.some((k) => overlaps(k, r)))) {
      continue;
    }
    kept.push(label);
    for (const c of cells) {
      const list = grid.get(c);
      if (list === undefined) {
        grid.set(c, [r]);
      } else {
        list.push(r);
      }
    }
  }
  return kept;
}
