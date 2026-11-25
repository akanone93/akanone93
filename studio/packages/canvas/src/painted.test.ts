// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import type { CanvasKit, Surface } from "canvaskit-wasm";
import { loadCanvasKit } from "../test-support/canvaskit.ts";
import { blendModeOf, drawPainted, fitImage } from "./painted.ts";
import type { RectSpec } from "./protocol.ts";

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

describe("[AST-002] an image's colour, as Flutter's Image blends it", () => {
  let ck: CanvasKit;
  beforeAll(async () => {
    ck = await loadCanvasKit();
  });

  /** The colour of the middle pixel of a 4×4 opaque white image drawn with `fill`'s tint. */
  function middle(fill: Partial<NonNullable<RectSpec["imageFit"]>>): number[] {
    const surface = ck.MakeSurface(4, 4) as Surface;
    const white = ck.MakeImage(
      {
        width: 4,
        height: 4,
        colorType: ck.ColorType.RGBA_8888,
        alphaType: ck.AlphaType.Unpremul,
        colorSpace: ck.ColorSpace.SRGB,
      },
      new Uint8Array(4 * 4 * 4).fill(255),
      16,
    );
    if (white === null) {
      throw new Error("no image");
    }
    const paint = new ck.Paint();
    const spec: RectSpec = {
      kind: "rect",
      id: "i",
      x: 0,
      y: 0,
      w: 4,
      h: 4,
      radius: 0,
      z: 1,
      image: "k",
      imageFit: { fit: "fill", alignX: 0, alignY: 0, ...fill },
    };
    drawPainted(ck, surface.getCanvas(), spec, paint, () => white);
    const shot = surface.makeImageSnapshot();
    const px = shot.readPixels(2, 2, {
      width: 1,
      height: 1,
      colorType: ck.ColorType.RGBA_8888,
      alphaType: ck.AlphaType.Unpremul,
      colorSpace: ck.ColorSpace.SRGB,
    }) as Uint8Array;
    for (const o of [shot, white, paint, surface]) {
      o.delete();
    }
    return [...px];
  }

  test("a colour tints a one-colour picture (srcIn); without one it is drawn as it is", () => {
    expect(middle({})).toEqual([255, 255, 255, 255]);
    expect(middle({ tint: 0xcc2200ff })).toEqual([0xcc, 0x22, 0x00, 255]);
    // dst keeps the picture, whatever the colour.
    expect(middle({ tint: 0xcc2200ff, blend: "dst" })).toEqual([255, 255, 255, 255]);
  });

  test("a blend mode is Flutter's name in CanvasKit's case; an unknown one is srcIn", () => {
    expect(blendModeOf(ck, "srcATop")).toBe(ck.BlendMode.SrcATop);
    expect(blendModeOf(ck, "multiply")).toBe(ck.BlendMode.Multiply);
    expect(blendModeOf(ck, undefined)).toBe(ck.BlendMode.SrcIn);
    expect(blendModeOf(ck, "nope")).toBe(ck.BlendMode.SrcIn);
  });
});
