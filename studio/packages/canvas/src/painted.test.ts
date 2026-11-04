// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { fitImage } from "./painted.ts";

const box = { x: 10, y: 20, w: 200, h: 100 };

describe("[AST-002] an image fitted in its box, as Flutter's applyBoxFit and paintImage fit it", () => {
  test("cover crops the image to fill the box; contain letterboxes it; fill stretches it", () => {
    // A square image in a 2:1 box.
    expect(fitImage("cover", 100, 100, box, 0, 0)).toEqual({ src: [0, 25, 100, 75], dst: [10, 20, 210, 120] });
    expect(fitImage("contain", 100, 100, box, 0, 0)).toEqual({ src: [0, 0, 100, 100], dst: [60, 20, 160, 120] });
    expect(fitImage("fill", 100, 100, box, 0, 0)).toEqual({ src: [0, 0, 100, 100], dst: [10, 20, 210, 120] });
  });

  test("alignment moves the fitted image in the box and the crop in the image", () => {
    expect(fitImage("contain", 100, 100, box, -1, 0).dst).toEqual([10, 20, 110, 120]);
    expect(fitImage("contain", 100, 100, box, 1, 0).dst).toEqual([110, 20, 210, 120]);
    expect(fitImage("cover", 100, 100, box, 0, -1).src).toEqual([0, 0, 100, 50]);
  });

  test("scaleDown keeps a small image at its size and shrinks a large one; none never scales", () => {
    expect(fitImage("scaleDown", 40, 20, box, 0, 0).dst).toEqual([90, 60, 130, 80]);
    expect(fitImage("scaleDown", 400, 400, box, 0, 0).dst).toEqual([60, 20, 160, 120]);
    expect(fitImage("none", 400, 400, box, 0, 0)).toEqual({ src: [100, 150, 300, 250], dst: [10, 20, 210, 120] });
    expect(fitImage("fitWidth", 100, 100, box, 0, 0).dst).toEqual([10, 20, 210, 120]);
    expect(fitImage("fitHeight", 100, 100, box, 0, 0).dst).toEqual([60, 20, 160, 120]);
  });
});
