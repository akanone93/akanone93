// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { finishResize } from "./select/gesture.ts";
import {
  draggedSides,
  gapRecipe,
  paddingRecipe,
  pixels,
  readSides,
  snapSize,
  snapSpacing,
  writeSides,
} from "./spacing-handles.ts";

const dims = { "space.sm": 8, "space.md": 12, "space.lg": 24, "radius.card": 16 };

describe("PC-18 padding and gap handles [STU-067]", () => {
  test("a padding prop reads as four sides in any of its shapes, start and end by direction", () => {
    expect(readSides({ all: 16 }, false).sides).toEqual({ top: 16, right: 16, bottom: 16, left: 16 });
    expect(readSides({ vertical: 8, horizontal: { $token: "space.md" } }, false).sides).toEqual({
      top: 8,
      bottom: 8,
      left: { $token: "space.md" },
      right: { $token: "space.md" },
    });
    expect(readSides({ start: 4, end: 20 }, true)).toEqual({
      sides: { top: 0, bottom: 0, left: 20, right: 4 },
      directional: true,
    });
    expect(readSides([1, 2, 3, 4], false).sides).toEqual({ left: 1, top: 2, right: 3, bottom: 4 });
    expect(pixels({ $token: "space.lg" }, dims)).toBe(24);
  });

  test("four sides are written in the shortest shape the document uses", () => {
    expect(writeSides({ top: 8, right: 8, bottom: 8, left: 8 }, false, false)).toEqual({ all: 8 });
    expect(writeSides({ top: 8, right: 16, bottom: 8, left: 16 }, false, false)).toEqual({
      vertical: 8,
      horizontal: 16,
    });
    expect(writeSides({ top: 8, right: 4, bottom: 8, left: 16 }, true, false)).toEqual({
      top: 8,
      bottom: 8,
      start: 16,
      end: 4,
    });
    expect(writeSides({ top: 1, right: 2, bottom: 3, left: 4 }, false, false)).toEqual({
      top: 1,
      right: 2,
      bottom: 3,
      left: 4,
    });
  });

  test("a dragged value snaps to a spacing token near it, else to a whole pixel; a size to any dimension token", () => {
    expect(snapSpacing(13.2, dims, 4)).toEqual({ $token: "space.md" });
    expect(snapSpacing(18.4, dims, 4)).toBe(18);
    expect(snapSpacing(-5, dims, 4)).toBe(0);
    expect(snapSpacing(16.5, dims, 4)).toBe(17);
    expect(snapSize(15, dims, 4)).toBe(16);
    expect(snapSize(41.6, dims, 4)).toBe(42);
    expect(draggedSides("top", false, false)).toEqual(["top"]);
    expect(draggedSides("left", true, false)).toEqual(["left", "right"]);
    expect(draggedSides("left", false, true)).toHaveLength(4);
  });

  test("the recipes write a widget's padding and its gap", () => {
    const root: { id: string; type: string; props: Record<string, unknown>; children: object[] } = {
      id: "p",
      type: "Padding",
      props: { padding: { all: 8 } },
      children: [{ id: "c", type: "Column" }],
    };
    paddingRecipe("p", { top: { $token: "space.lg" }, right: 8, bottom: 8, left: 8 }, false, false)(root);
    expect(root.props["padding"]).toEqual({ top: { $token: "space.lg" }, right: 8, bottom: 8, left: 8 });
    gapRecipe("c", 12)(root);
    expect(root.children[0]).toMatchObject({ props: { spacing: 12 } });
  });

  test("a resize snaps its width and height through the canvas's snapping", () => {
    const edits: unknown[] = [];
    const root = { id: "b", type: "SizedBox", props: { width: 100, height: 50 } };
    finishResize(
      {
        index: { parent: new Map(), children: new Map(), depth: new Map(), type: new Map() },
        edit: (_doc, recipe) => {
          recipe(root);
          edits.push(root.props);
        },
        docOf: () => "page",
        boxOf: () => ({ l: 0, t: 0, r: 100, b: 50 }),
        typeOf: () => "SizedBox",
        snapSize: (v) => snapSize(v, { "size.tap": 48 }, 4),
      },
      "b",
      { x: 3.4, y: -3 },
    );
    expect(root.props).toEqual({ width: 103, height: 48 });
  });
});
