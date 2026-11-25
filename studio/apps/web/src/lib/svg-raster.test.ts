// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { rasterSize, sizedSvg, svgSize } from "./svg-raster.ts";

describe("[CMP-031] an SVG asset drawn on the canvas (build log rows 25, 39)", () => {
  test("the size an SVG declares: width and height, the viewBox's proportion for a missing one, or the viewBox", () => {
    expect(svgSize('<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24px">')).toEqual({
      width: 24,
      height: 24,
    });
    expect(svgSize("<svg width='48' viewBox='0 0 24 12'><path/></svg>")).toEqual({ width: 48, height: 24 });
    expect(svgSize('<?xml version="1.0"?><svg viewBox="0,0,32,16"></svg>')).toEqual({ width: 32, height: 16 });
    // A stroke-width is not the width; a percentage is no size.
    expect(svgSize('<svg stroke-width="2" width="100%"></svg>')).toBeNull();
    expect(svgSize("<html></html>")).toBeNull();
  });

  test("drawn at four times its size, the longer side between 256 and 1024 pixels", () => {
    expect(rasterSize({ width: 24, height: 24 })).toEqual({ width: 256, height: 256 });
    expect(rasterSize({ width: 100, height: 50 })).toEqual({ width: 400, height: 200 });
    expect(rasterSize({ width: 2000, height: 1000 })).toEqual({ width: 1024, height: 512 });
    expect(rasterSize(null)).toEqual({ width: 256, height: 256 });
  });

  test("the root is sized to the raster and keeps a viewBox, so the drawing scales; the rest is untouched", () => {
    const icon = '<svg width="24" height="24" stroke-width="2"><rect width="10" height="10"/></svg>';
    expect(sizedSvg(icon, { width: 24, height: 24 }, { width: 256, height: 256 })).toBe(
      '<svg width="256" height="256" viewBox="0 0 24 24" stroke-width="2"><rect width="10" height="10"/></svg>',
    );
    const boxed = '<svg viewBox="0 0 10 5" width="10"></svg>';
    expect(sizedSvg(boxed, { width: 10, height: 5 }, { width: 256, height: 128 })).toBe(
      '<svg width="256" height="128" viewBox="0 0 10 5"></svg>',
    );
    expect(sizedSvg("<p>no svg</p>", null, { width: 1, height: 1 })).toBeNull();
  });
});
