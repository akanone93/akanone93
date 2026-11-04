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
import { GestureTracker, guideLines, WHEEL_REST_MS } from "./use-canvas-gestures.ts";

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

  test("the canvas document round-trips, is written in a stable order and holds only document ids", () => {
    const a = "01a0f752-7d75-72a3-9a43-472f6fbc3b6a";
    const b = "01a0f752-7d75-72a3-9a43-472f6fbc3b6b";
    const saved = readSavedCanvas(
      canvasDocument("id", {
        positions: new Map([
          [b, { x: 1.4, y: 2 }],
          ["native:camera", { x: 9, y: 9 }],
          [a, { x: 3, y: 4 }],
        ]),
        frames: [{ id: "f", label: "Payments", x: 0, y: 0, width: 300, height: 200 }],
      }),
    );
    // A native box has no document id, which the canvas schema refuses: it is left out, not the whole save.
    expect([...saved.positions.keys()]).toEqual([a, b]);
    expect(saved.positions.get(b)).toEqual({ x: 1, y: 2 });
    expect(saved.frames[0]?.label).toBe("Payments");
    expect(readSavedCanvas("nonsense").positions.size).toBe(0);
  });

  test("plugin keys are lower-kebab slugs, unique among the taken ones", () => {
    expect(pluginKeyFrom("Loan Products!", new Set())).toBe("loan-products");
    expect(pluginKeyFrom("Loan Products", new Set(["loan-products"]))).toBe("loan-products-2");
    expect(pluginKeyFrom("123", new Set())).toBe("plugin");
  });
});

describe("snap guides [STU-032]", () => {
  test("a guide runs from the dragged box to the boxes it aligns with, a little past both", () => {
    const moved = { x: 0, y: 300 };
    const lines = guideLines(
      [
        { axis: "x", at: 0 },
        { axis: "y", at: 300 + BOX.height },
      ],
      moved,
      [
        { x: 0, y: 0 },
        { x: 500, y: 300 },
        { x: 900, y: 900 },
      ],
    );
    expect(lines).toEqual([
      { axis: "x", at: 0, from: -12, to: 300 + BOX.height + 12 },
      { axis: "y", at: 300 + BOX.height, from: -12, to: 500 + BOX.width + 12 },
    ]);
  });
});

describe("view gestures reach the worker (M-12) [STU-032]", () => {
  test("a burst of wheel steps is one gesture, ended once the wheel rests; a new kind ends the old one", async () => {
    const calls: string[] = [];
    const surface = { gesture: (kind: string, active: boolean) => calls.push(`${kind}:${active}`) };
    const tracker = new GestureTracker();
    const target = surface as unknown as Parameters<GestureTracker["wheel"]>[0];
    tracker.wheel(target, "zoom");
    tracker.wheel(target, "zoom");
    tracker.wheel(target, "zoom");
    expect(calls).toEqual(["zoom:true"]);
    tracker.wheel(target, "pan");
    expect(calls).toEqual(["zoom:true", "zoom:false", "pan:true"]);
    await new Promise((resolve) => setTimeout(resolve, WHEEL_REST_MS + 40));
    expect(calls).toEqual(["zoom:true", "zoom:false", "pan:true", "pan:false"]);
  });
});
