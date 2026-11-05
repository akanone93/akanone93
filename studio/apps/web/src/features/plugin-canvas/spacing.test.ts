// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { spacingBands } from "./interaction-overlay.tsx";

const box = (l: number, t: number, r: number, b: number) => ({ l, t, r, b });

describe("PC-16 a selected widget's padding and gaps [STU-067]", () => {
  test("one child inside: the four bands of padding with their sizes", () => {
    const bands = spacingBands(box(0, 0, 100, 60), [box(16, 8, 84, 52)]);
    expect(bands.map((b) => [b.kind, b.value])).toEqual([
      ["padding", 8],
      ["padding", 8],
      ["padding", 16],
      ["padding", 16],
    ]);
    expect(spacingBands(box(0, 0, 100, 60), [box(0, 0, 100, 60)])).toEqual([]);
  });

  test("children in a column or a row: the gaps between them; a stack has none", () => {
    const column = spacingBands(box(0, 0, 100, 200), [box(0, 0, 100, 40), box(0, 52, 80, 92), box(0, 92, 100, 130)]);
    expect(column).toEqual([{ kind: "gap", value: 12, box: box(0, 40, 100, 52) }]);
    const row = spacingBands(box(0, 0, 300, 40), [box(200, 0, 260, 40), box(0, 0, 80, 40)]);
    expect(row.map((b) => b.value)).toEqual([120]);
    expect(spacingBands(box(0, 0, 100, 100), [box(0, 0, 100, 100), box(10, 10, 50, 50)])).toEqual([]);
  });
});
