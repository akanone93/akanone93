// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { cornersShown, tagClear } from "./overlay-layout.ts";

describe("[STU-067] overlay marks that do not cover each other", () => {
  test("nested widgets on one corner: the innermost keeps its badges", () => {
    const shown = cornersShown(
      [
        { id: "column", x: 300, y: 100, area: 90_000 },
        { id: "text", x: 298, y: 101, area: 2_000 },
        { id: "far", x: 120, y: 400, area: 5_000 },
      ],
      new Set(),
    );
    expect([...shown].sort()).toEqual(["far", "text"]);
  });

  test("the hovered or selected widget takes its corner, so one cluster still shows there", () => {
    const corners = [
      { id: "column", x: 300, y: 100, area: 90_000 },
      { id: "text", x: 298, y: 101, area: 2_000 },
    ];
    expect([...cornersShown(corners, new Set(["column"]))]).toEqual(["column"]);
    // Two focused widgets on one corner (a hovered child of the selection) both show.
    expect([...cornersShown(corners, new Set(["column", "text"]))].sort()).toEqual(["column", "text"]);
  });

  test("a hovered widget's name tag steps aside when it would sit on the selected widget's", () => {
    expect(tagClear({ x: 10, y: 10 }, { x: 14, y: 12 })).toBe(false);
    expect(tagClear({ x: 10, y: 80 }, { x: 14, y: 12 })).toBe(true);
    expect(tagClear({ x: 300, y: 10 }, { x: 14, y: 12 })).toBe(true);
    expect(tagClear({ x: 10, y: 10 }, undefined)).toBe(true);
  });
});
