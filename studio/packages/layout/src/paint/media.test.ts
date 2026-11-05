// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { colorSchemeFromSeed } from "../material-color/scheme.ts";
import { controlPainters } from "./controls.ts";
import { mediaPaint } from "./media.ts";
import { assetOf } from "./media-source.ts";
import { argbToRgba, type PaintTheme } from "./paint.ts";

const scheme = colorSchemeFromSeed(0xff6750a4, false);
const theme: PaintTheme = { scheme };
const role = (r: keyof typeof scheme) => argbToRgba(scheme[r] as number);
const r = { x: 0, y: 0, w: 216, h: 120 };

describe("[WGT-021] [WGT-031] charts from their data, previews of what the device shows", () => {
  test("a bar chart: a bar per value in its series' colour, scaled to the largest, over a baseline", () => {
    const paint = mediaPaint(
      { type: "BarChart", props: { series: [{ values: [1, 2] }, { values: [2, 4] }], labels: ["a", "b"] } },
      r,
      theme,
    );
    const bars = paint?.boxes.slice(1) ?? [];
    expect(bars.map((b) => b.fill)).toEqual([role("primary"), role("primary"), role("tertiary"), role("tertiary")]);
    // The plot: 120 high, 8 above, 20 for the labels below.
    expect(bars[3]?.h).toBeCloseTo(92, 5);
    expect(bars[0]?.h).toBeCloseTo(23, 5);
    expect(paint?.texts.map((t) => t.text)).toEqual(["a", "b"]);
  });

  test("a line chart joins its values; a sparkline too, without dots", () => {
    const line = mediaPaint({ type: "LineChart", props: { series: [{ values: [1, 3, 2] }] } }, r, theme);
    expect(line?.lines).toHaveLength(2);
    expect(line?.boxes.filter((b) => b.oval === true)).toHaveLength(3);
    const spark = mediaPaint({ type: "Sparkline", props: { values: [1, 2, 3, 4] } }, r, theme);
    expect(spark?.lines).toHaveLength(3);
    expect(spark?.boxes).toEqual([]);
  });

  test("a pie chart's wedges sweep the whole circle from 12 o'clock", () => {
    const pie = mediaPaint({ type: "PieChart", props: { slices: [{ value: 1 }, { value: 3 }] } }, r, theme);
    expect(pie?.boxes.map((b) => b.arc)).toEqual([
      { start: -90, sweep: 90 },
      { start: 0, sweep: 270 },
    ]);
  });

  test("a scanner's viewfinder, a video's play glyph, a native slot's outline; a progress value's arc", () => {
    expect(mediaPaint({ type: "QrScanner", props: { overlayHint: "Scan" } }, r, theme)?.texts[0]?.text).toBe("Scan");
    expect(mediaPaint({ type: "VideoPlayer", props: {} }, r, theme)?.glyphs[0]?.name).toBe("play_circle");
    expect(mediaPaint({ type: "NativeSlot", props: {} }, r, theme)?.glyphs[0]?.name).toBe("extension");
    expect(mediaPaint({ type: "Text" }, r, theme)).toBeUndefined();
    const progress = controlPainters.get("CircularProgressIndicator")?.({ props: { value: 0.5 } }, r, theme);
    expect(progress?.[0]?.arc).toEqual({ start: -90, sweep: 180, width: 4 });
    expect(assetOf({ asset: { $asset: "a" } })).toBe("a");
    expect(assetOf({ url: "https://x" })).toBeUndefined();
  });
});
