// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  animationOf,
  cubicPointsOf,
  curveAt,
  curveValueOf,
  frameAt,
  heroMatches,
  setAnimationRecipe,
  transitionFrame,
  withCurve,
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

  // Build log row 48: a curve may be a cubic Bézier's control points instead of a name.
  test("a cubic Bézier curve runs as Flutter's Cubic does, and only with x1 and x2 from 0 to 1", () => {
    expect(curveAt([0.4, 0, 0.2, 1], 0.5)).toBeCloseTo(curveAt("fastOutSlowIn", 0.5), 6);
    expect(curveAt([0.2, 0, 0, 1], 0)).toBeCloseTo(0, 3);
    expect(curveAt([0.2, 0, 0, 1], 1)).toBeCloseTo(1, 3);
    expect(cubicPointsOf([0.2, 0, 0, 1])).toEqual([0.2, 0, 0, 1]);
    expect(cubicPointsOf([1.2, 0, 0, 1])).toBeUndefined();
    expect(cubicPointsOf([0.2, 0, 1])).toBeUndefined();
    expect(curveValueOf({ curve: "easeIn", cubic: [0.2, 0, 0, 1] })).toEqual([0.2, 0, 0, 1]);
    expect(curveValueOf({ curve: "easeIn" })).toBe("easeIn");
  });

  test("a curve is written as a name or as control points, the other removed, in one edit", () => {
    expect(withCurve({ kind: "fade", curve: "easeIn" }, [0.2, 0, 0, 1])).toEqual({
      kind: "fade",
      cubic: [0.2, 0, 0, 1],
    });
    expect(withCurve({ kind: "fade", cubic: [0.2, 0, 0, 1] }, "easeOut")).toEqual({ kind: "fade", curve: "easeOut" });
    const root = { id: "r", type: "Column", children: [{ id: "c", type: "Card" }] as Record<string, unknown>[] };
    setAnimationRecipe("c", "durationMs", 200)(root);
    setAnimationRecipe("c", "curve", "easeIn")(root);
    setAnimationRecipe("c", "curve", [0.05, 0.7, 0.1, 1])(root);
    expect(animationOf(root.children[0])).toEqual({ durationMs: 200, cubic: [0.05, 0.7, 0.1, 1] });
    setAnimationRecipe("c", "curve", "decelerate")(root);
    expect(animationOf(root.children[0])).toEqual({ durationMs: 200, curve: "decelerate" });
    const frame = frameAt({ kind: "fade", durationMs: 100, cubic: [0, 0, 1, 1] }, "enter", 50, { w: 10, h: 10 });
    expect(frame?.opacity).toBeCloseTo(0.5, 2);
  });
});
