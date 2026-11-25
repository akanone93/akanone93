// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { colorSchemeFromSeed } from "../material-color/scheme.ts";
import { argbToRgba, type PaintTheme } from "./paint.ts";
import { pickerPaint, securePinPadPaint } from "./secure-pickers.ts";

const scheme = colorSchemeFromSeed(0xff6750a4, false);
const theme: PaintTheme = { scheme };

describe("the likenesses of the secure pin pad and the pickers (#127)", () => {
  test("a SecurePinPad draws its rings and a keypad of digits with delete, mirrored right to left", () => {
    const r = { x: 0, y: 0, w: 240, h: 282 };
    const pad = securePinPadPaint({ length: 6 }, r, theme);
    expect(pad.boxes).toHaveLength(6);
    expect(pad.boxes.every((b) => b.oval === true && b.border !== undefined)).toBe(true);
    expect(pad.texts.map((t) => t.text)).toEqual(["1", "2", "3", "4", "5", "6", "7", "8", "9", "0"]);
    expect(pad.glyphs.map((g) => g.name)).toEqual(["backspace"]);
    // Left to right, 1 is at the start; right to left, at the right edge.
    expect(pad.texts[0]?.x).toBe(0);
    const rtl = securePinPadPaint({ length: 6 }, r, { ...theme, rtl: true });
    expect(rtl.texts[0]?.x).toBe(160);
    expect(rtl.glyphs[0]?.x).toBeLessThan(80);
  });

  test("a CurrencyPicker draws a search field over the currencies, the selected one in primary with a check", () => {
    const r = { x: 0, y: 0, w: 360, h: 360 };
    const picker = pickerPaint({ value: "EUR", currencies: ["USD", "EUR"], searchHint: "Find" }, r, theme, "currency");
    const texts = picker.texts.map((t) => t.text);
    expect(texts[0]).toBe("Find");
    expect(texts).toContain("Euro");
    expect(texts).toContain("US Dollar");
    // Sorted by name: Euro before US Dollar.
    expect(texts.indexOf("Euro")).toBeLessThan(texts.indexOf("US Dollar"));
    expect(picker.glyphs.map((g) => g.name)).toEqual(["search", "check"]);
    const euro = picker.texts.find((t) => t.text === "Euro");
    expect(euro?.color).toEqual(argbToRgba(scheme.primary as number));
    expect(picker.boxes[0]?.border).toBeDefined();
  });

  test("a picker lists as many tiles as fit, and a CountryPicker names its territories", () => {
    const small = pickerPaint({}, { x: 0, y: 0, w: 360, h: 72 + 72 + 10 }, theme, "currency");
    expect(small.texts.length).toBe(1 + 3);
    const countries = pickerPaint({}, { x: 0, y: 0, w: 360, h: 360 }, theme, "country");
    expect(countries.texts.map((t) => t.text)).toContain("Austria");
  });
});
