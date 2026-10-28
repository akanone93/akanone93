// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  autoLayout,
  BOX,
  canvasDocument,
  fitViewport,
  marquee,
  placeBoxes,
  pluginKeyFrom,
  readSavedCanvas,
  select,
  snap,
} from "./app-canvas.model.ts";

describe("app canvas model [STU-030] [STU-032]", () => {
  test("auto layout ranks linked plugins left to right and is deterministic", () => {
    const links = [
      { from: "a", to: "b" },
      { from: "b", to: "c" },
    ];
    const first = autoLayout(["a", "b", "c"], links);
    expect(first.get("a")?.x ?? 0).toBeLessThan(first.get("b")?.x ?? 0);
    expect(first.get("b")?.x ?? 0).toBeLessThan(first.get("c")?.x ?? 0);
    expect(autoLayout(["a", "b", "c"], links)).toEqual(first);
  });

  test("saved positions win; only unsaved plugins are laid out", () => {
    const saved = new Map([["a", { x: 500, y: 40 }]]);
    const boxes = placeBoxes(
      [
        { id: "a", key: "a" },
        { id: "b", key: "b" },
      ],
      saved,
      [],
    );
    expect(boxes[0]).toMatchObject({ x: 500, y: 40 });
    expect(boxes[1]?.key).toBe("b");
  });

  test("snapping aligns edges and centres within range and reports guides", () => {
    const s = snap({ x: 103, y: 400 }, [{ x: 100, y: 0 }]);
    expect(s.point).toEqual({ x: 100, y: 400 });
    expect(s.guides).toEqual([{ axis: "x", at: 100 }]);
    const centre = snap({ x: 0, y: 100 + BOX.height / 2 - BOX.height / 2 + 4 }, [{ x: 0, y: 100 }]);
    expect(centre.point.y).toBe(100);
  });

  test("selection toggles with the additive key and a marquee selects what it touches", () => {
    const one = select(new Set(), "a", false);
    const two = select(one, "b", true);
    expect([...two]).toEqual(["a", "b"]);
    expect([...select(two, "a", true)]).toEqual(["b"]);
    const boxes = [
      { id: "a", key: "a", x: 0, y: 0 },
      { id: "b", key: "b", x: 1000, y: 0 },
    ];
    expect([...marquee(boxes, { x: -10, y: -10, width: 50, height: 50 })]).toEqual(["a"]);
  });

  test("zoom to fit centres every box in the view within the zoom range", () => {
    const v = fitViewport(
      [
        { x: 0, y: 0 },
        { x: 2000, y: 0 },
      ],
      { width: 1000, height: 600 },
    );
    expect(v.z).toBeLessThan(1);
    expect(fitViewport([], { width: 1000, height: 600 })).toEqual({ x: 0, y: 0, z: 1 });
  });

  test("the canvas document round-trips and is written in a stable order", () => {
    const saved = readSavedCanvas(
      canvasDocument("id", {
        positions: new Map([
          ["b", { x: 1.4, y: 2 }],
          ["a", { x: 3, y: 4 }],
        ]),
        frames: [{ id: "f", label: "Payments", x: 0, y: 0, width: 300, height: 200 }],
      }),
    );
    expect([...saved.positions.keys()]).toEqual(["a", "b"]);
    expect(saved.positions.get("b")).toEqual({ x: 1, y: 2 });
    expect(saved.frames[0]?.label).toBe("Payments");
    expect(readSavedCanvas("nonsense").positions.size).toBe(0);
  });

  test("plugin keys are lower-kebab slugs, unique among the taken ones", () => {
    expect(pluginKeyFrom("Loan Products!", new Set())).toBe("loan-products");
    expect(pluginKeyFrom("Loan Products", new Set(["loan-products"]))).toBe("loan-products-2");
    expect(pluginKeyFrom("123", new Set())).toBe("plugin");
  });
});
