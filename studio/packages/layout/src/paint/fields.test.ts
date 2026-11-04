// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { colorSchemeFromSeed } from "../material-color/scheme.ts";
import { fieldModel, fieldPaint, fieldSceneProps, searchBarPaint } from "./fields.ts";
import { argbToRgba, type PaintTheme } from "./paint.ts";

const scheme = colorSchemeFromSeed(0xff6750a4, false);
const theme: PaintTheme = { scheme };
const role = (r: keyof typeof scheme) => argbToRgba(scheme[r] as number);
const field = { x: 0, y: 0, w: 300, h: 80 };
const parts: Record<string, { x: number; y: number; w: number; h: number }> = {
  "f#input": { x: 16, y: 24, w: 268, h: 24 },
  "f#subtext": { x: 16, y: 60, w: 268, h: 16 },
  "f#suffixIcon": { x: 252, y: 16, w: 48, h: 24 },
};
const rectOf = (id: string) => parts[id];

describe("[STU-005] [STA-020] Material 3 text fields as Flutter's input decorator paints them", () => {
  test("a TextField's decoration and value are read as the scene's flat props", () => {
    const model = fieldModel({
      value: "120",
      decoration: { labelText: "Amount", filled: true, border: { kind: "outline" }, prefixIcon: { name: "euro" } },
    });
    expect(model).toMatchObject({ label: "Amount", value: "120", filled: true, border: "outline" });
    expect(fieldSceneProps(model)).toEqual({ labelText: "Amount", text: "120", filled: true, border: "outline" });
  });

  test("an empty field: the label rests in the input line; an underline in onSurfaceVariant", () => {
    const paint = fieldPaint(fieldModel({ decoration: { labelText: "Name" } }), "f", field, theme, rectOf);
    expect(paint.texts).toEqual([
      expect.objectContaining({ text: "Name", x: 16, y: 24, color: role("onSurfaceVariant") }),
    ]);
    expect(paint.boxes[0]?.border).toEqual({ kind: "sides", bottom: { color: role("onSurfaceVariant"), width: 1 } });
  });

  test("with a value the label floats in bodySmall; filled fields fill surfaceContainerHighest", () => {
    const model = fieldModel({
      value: "Ada",
      decoration: { labelText: "Name", filled: true, helperText: "Your name" },
    });
    const paint = fieldPaint(model, "f", field, theme, rectOf);
    expect(paint.boxes[0]).toMatchObject({ h: 56, fill: role("surfaceContainerHighest"), radii: [4, 4, 0, 0] });
    expect(paint.texts.map((t) => [t.text, t.style.size])).toEqual([
      ["Ada", 16],
      ["Name", 12],
      ["Your name", 12],
    ]);
  });

  test("an error turns the outline, the label, the subtext and the suffix icon to error", () => {
    const model = fieldModel({
      value: "x",
      decoration: {
        labelText: "IBAN",
        border: { kind: "outline" },
        errorText: "Check it",
        suffixIcon: { name: "error" },
      },
    });
    const paint = fieldPaint(model, "f", field, theme, rectOf);
    expect(paint.boxes[0]?.border).toEqual({ kind: "all", side: { color: role("error"), width: 1 } });
    expect(paint.texts.filter((t) => t.color === role("error")).map((t) => t.text)).toEqual(["IBAN", "Check it"]);
    expect(paint.glyphs).toEqual([expect.objectContaining({ name: "error", color: role("error") })]);
  });

  test("a search bar is a 56 pixel pill with a search icon and its hint", () => {
    const paint = searchBarPaint({ hintText: "Search" }, { x: 0, y: 0, w: 360, h: 56 }, theme);
    expect(paint.boxes[0]).toMatchObject({ radii: [28, 28, 28, 28], fill: role("surfaceContainerHigh") });
    expect(paint.texts[0]).toMatchObject({ text: "Search", x: 56, color: role("onSurfaceVariant") });
    expect(paint.glyphs[0]).toMatchObject({ name: "search", x: 16 });
  });
});
