// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { colorSchemeFromSeed, type PaintTheme } from "@plux/layout";
import { a11yMarksOf, contrast, focusOrder, interactive, lowContrast, smallTarget } from "./a11y-marks.ts";

const theme: PaintTheme = { scheme: colorSchemeFromSeed(0xff6750a4, false) };
const button = (id: string, order?: number) => ({
  id,
  type: "FilledButton",
  ...(order === undefined ? {} : { semantics: { focusOrder: order } }),
  slots: { child: { id: `${id}t`, type: "Text", props: { data: id } } },
});

describe("PC-31 the Accessibility view [A11Y-003] [A11Y-004] [A11Y-006]", () => {
  test("focus moves through interactive widgets, explicit focus orders first, then in reading order", () => {
    const root = {
      id: "col",
      type: "Column",
      children: [button("a"), button("b", 1), button("c"), { id: "t", type: "Text" }],
    };
    expect([...focusOrder(root)]).toEqual([
      ["b", 1],
      ["a", 2],
      ["c", 3],
    ]);
    expect(interactive("Text")).toBe(false);
  });

  test("touch targets below the platform's minimum are flagged", () => {
    expect(smallTarget({ w: 48, h: 48 }, 48)).toBe(false);
    expect(smallTarget({ w: 40, h: 48 }, 48)).toBe(true);
    expect(smallTarget({ w: 44.2, h: 44 }, 44)).toBe(false);
  });

  test("text colour over the nearest background is checked against 4.5:1, 3:1 for large text", () => {
    expect(contrast(0x000000ff, 0xffffffff)).toBeCloseTo(21, 3);
    expect(contrast(0x777777ff, 0xffffffff)).toBeCloseTo(4.48, 1);
    const root = {
      id: "s",
      type: "Scaffold",
      slots: {
        body: {
          id: "box",
          type: "Container",
          props: { color: "#FFFFFFFF" },
          slots: {
            child: {
              id: "col",
              type: "Column",
              children: [
                { id: "pale", type: "Text", props: { data: "x", style: { color: "#AAAAAAFF" } } },
                { id: "big", type: "Text", props: { data: "x", style: { color: "#888888FF", fontSize: 28 } } },
                { id: "ok", type: "Text", props: { data: "x", style: { color: "#333333FF" } } },
                { id: "own", type: "Text", props: { data: "x" } },
              ],
            },
          },
        },
      },
    };
    const low = lowContrast(root, theme);
    expect([...low.keys()]).toEqual(["pale"]);
    expect(low.get("pale")?.needed).toBe(4.5);
    const marks = a11yMarksOf(
      [{ root }],
      theme,
      new Map([
        ["b", "FilledButton"],
        ["t", "Text"],
      ]),
      "cupertino",
    );
    expect(marks.min).toBe(44);
    expect([...marks.targets]).toEqual(["b"]);
  });
});
