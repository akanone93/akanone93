// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  animationOf,
  curveAt,
  frameAt,
  heroMatches,
  setAnimationRecipe,
  transitionFrame,
  withMotion,
} from "./motion.ts";
import { nodeIdOfHit } from "./select/tree.ts";

describe("PC-29 motion on the canvas [ANI-001] [ANI-003] [ANI-004]", () => {
  test("curves run from 0 to 1 as Flutter's Curves do", () => {
    for (const c of [
      "linear",
      "easeIn",
      "easeOut",
      "easeInOut",
      "fastOutSlowIn",
      "decelerate",
      "bounceIn",
      "bounceOut",
      "elasticOut",
      "overshoot",
    ]) {
      expect(curveAt(c, 0)).toBeCloseTo(0, 3);
      expect(curveAt(c, 1)).toBeCloseTo(1, 3);
    }
    expect(curveAt("linear", 0.25)).toBe(0.25);
    expect(curveAt("easeIn", 0.5)).toBeLessThan(0.5);
    expect(curveAt("easeOut", 0.5)).toBeGreaterThan(0.5);
    expect(curveAt("fastOutSlowIn", 0.5)).toBeCloseTo(0.774, 2);
    expect(Math.max(...[0.6, 0.7, 0.8].map((t) => curveAt("overshoot", t)))).toBeGreaterThan(1);
  });

  test("an enter transition goes from its start to the widget at rest; an exit goes back, and ends", () => {
    const size = { w: 200, h: 40 };
    expect(transitionFrame("slideUp", 0, size)).toEqual({ opacity: 1, dx: 0, dy: 40, scale: 1 });
    expect(transitionFrame("slideLeft", 0.5, size).dx).toBe(100);
    expect(transitionFrame("scale", 0.25, size).scale).toBe(0.25);
    const fade = { kind: "fade", durationMs: 200, curve: "linear" };
    expect(frameAt(fade, "enter", 50, size)?.opacity).toBeCloseTo(0.25, 5);
    expect(frameAt(fade, "exit", 50, size)?.opacity).toBeCloseTo(0.75, 5);
    expect(frameAt(fade, "enter", 250, size)).toBeUndefined();
  });

  test("a played frame wraps only the drawn widget, which a click still selects", () => {
    const root = {
      id: "col",
      type: "Column",
      children: [
        { id: "a", type: "Text" },
        { id: "b", type: "Text" },
      ],
    };
    const drawn = withMotion(root, "b", { opacity: 0.5, dx: 0, dy: 12, scale: 1 }) as {
      children: Record<string, unknown>[];
    };
    expect(drawn.children[1]).toMatchObject({ id: "b~motion", type: "Opacity", props: { opacity: 0.5 } });
    expect(JSON.stringify(drawn.children[1])).toContain('"offset":{"dx":0,"dy":12}');
    expect(drawn.children[0]).toEqual({ id: "a", type: "Text" });
    expect(nodeIdOfHit("s/b~motion-move")).toBe("b");
    expect(root.children[1]).toEqual({ id: "b", type: "Text" });
  });

  test("animation keys are written and an empty animation removed; hero tags pair widgets across screens", () => {
    const root = { id: "r", type: "Column", children: [{ id: "img", type: "Image" }] as Record<string, unknown>[] };
    setAnimationRecipe("img", "hero", "avatar")(root);
    setAnimationRecipe("img", "enter", { kind: "fade" })(root);
    expect(animationOf(root.children[0])).toEqual({ hero: "avatar", enter: { kind: "fade" } });
    const other = { id: "p2", type: "Column", children: [{ id: "big", type: "Image", animation: { hero: "avatar" } }] };
    expect(heroMatches([root, other], "img")).toEqual(["big"]);
    expect(heroMatches([root, other], "r")).toEqual([]);
    setAnimationRecipe("img", "hero", undefined)(root);
    setAnimationRecipe("img", "enter", undefined)(root);
    expect("animation" in (root.children[0] as object)).toBe(false);
  });
});
