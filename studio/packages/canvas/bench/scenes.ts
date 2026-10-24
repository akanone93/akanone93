// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The benchmark scenes of ADR-0013 M-12: scene A, 200 boxes with titles and
 * 1,000 curved links in three styles; scene B, 100 screens with shadows,
 * cards and text. Deterministic (seeded) so runs compare.
 */

import type { ItemSpec, LinkSpec, Point } from "../src/protocol.ts";

const BOX_W = 160;
const BOX_H = 80;
const GAP_X = 240;
const GAP_Y = 160;
const COLUMNS = 20;
const BOX_COLORS = [0xffffffff, 0xe8f0feff, 0xfef7e0ff, 0xe6f4eaff];
const LINK_COLORS = [0x5f6368ff, 0x1a73e8ff, 0xd93025ff];

function prng(seed: number): () => number {
  let state = seed >>> 0;
  return () => {
    state = (Math.imul(state, 1664525) + 1013904223) >>> 0;
    return state / 2 ** 32;
  };
}

/** The top-left corner of box `index`. */
export function boxPosition(index: number): Point {
  return { x: (index % COLUMNS) * GAP_X, y: Math.floor(index / COLUMNS) * GAP_Y };
}

/** A link from the right edge of box `a` to the left edge of box `b`. */
export function linkSpec(id: string, a: Point, b: Point, style: number): LinkSpec {
  const from = { x: a.x + BOX_W, y: a.y + BOX_H / 2 };
  const to = { x: b.x, y: b.y + BOX_H / 2 };
  const bend = Math.max(40, Math.abs(to.x - from.x) / 2);
  return {
    kind: "link",
    id,
    from,
    to,
    c1: { x: from.x + bend, y: from.y },
    c2: { x: to.x - bend, y: to.y },
    stroke: { color: LINK_COLORS[style] ?? 0x000000ff, width: style === 0 ? 1.5 : 2 },
    z: 0,
  };
}

/** One link of scene A: its id, the two box indices and its style. */
export interface LinkRef {
  readonly id: string;
  readonly a: number;
  readonly b: number;
  readonly style: number;
}

/** Scene A: items plus the link table, so a drag can re-route links. */
export function sceneA(): { items: ItemSpec[]; links: LinkRef[] } {
  const random = prng(42);
  const items: ItemSpec[] = [];
  const links: LinkRef[] = [];
  for (let i = 0; i < 1000; i++) {
    const a = Math.floor(random() * 200);
    const b = (a + 1 + Math.floor(random() * 199)) % 200;
    const ref = { id: `link-${i}`, a, b, style: i % 3 };
    links.push(ref);
    items.push(linkSpec(ref.id, boxPosition(a), boxPosition(b), ref.style));
  }
  for (let i = 0; i < 200; i++) {
    const { x, y } = boxPosition(i);
    items.push({
      kind: "rect",
      id: `box-${i}`,
      label: `Action ${i}`,
      x,
      y,
      w: BOX_W,
      h: BOX_H,
      radius: 8,
      fill: BOX_COLORS[i % BOX_COLORS.length] ?? 0xffffffff,
      stroke: { color: 0xdadce0ff, width: 1 },
      z: 1,
    });
    items.push({
      kind: "text",
      id: `label-${i}`,
      x: x + 12,
      y: y + 12,
      w: BOX_W - 24,
      text: `Action ${i}`,
      size: 14,
      color: 0x202124ff,
      z: 2,
    });
  }
  return { items, links };
}

/** Scene B: 100 screens of 360 x 780 with a header, cards and text lines. */
export function sceneB(): ItemSpec[] {
  const items: ItemSpec[] = [];
  for (let s = 0; s < 100; s++) {
    const x = (s % 10) * 440;
    const y = Math.floor(s / 10) * 880;
    const frame = `screen-${s}`;
    items.push({
      kind: "frame",
      id: frame,
      name: `Screen ${s}`,
      x,
      y,
      w: 360,
      h: 780,
      radius: 24,
      fill: 0xffffffff,
      shadow: true,
      z: 0,
    });
    items.push({ kind: "rect", id: `${frame}-bar`, frame, x, y, w: 360, h: 64, radius: 0, fill: 0x1a73e8ff, z: 1 });
    for (let c = 0; c < 6; c++) {
      const cy = y + 88 + c * 112;
      items.push({
        kind: "rect",
        id: `${frame}-card-${c}`,
        frame,
        x: x + 16,
        y: cy,
        w: 328,
        h: 96,
        radius: 12,
        fill: 0xf1f3f4ff,
        z: 1,
      });
      items.push({
        kind: "text",
        id: `${frame}-title-${c}`,
        frame,
        x: x + 32,
        y: cy + 16,
        w: 296,
        text: `Card ${c} of screen ${s}`,
        size: 16,
        color: 0x202124ff,
        z: 2,
      });
    }
    items.push({
      kind: "text",
      id: `${frame}-heading`,
      frame,
      x: x + 16,
      y: y + 20,
      w: 328,
      text: `Screen ${s}`,
      size: 20,
      color: 0xffffffff,
      z: 2,
    });
  }
  return items;
}
