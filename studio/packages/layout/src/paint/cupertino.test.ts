// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { colorSchemeFromSeed } from "../material-color/scheme.ts";
import { cupertinoForeground, cupertinoPainterOf } from "./cupertino.ts";
import { argbToRgba, type PaintTheme } from "./paint.ts";

const light: PaintTheme = { scheme: colorSchemeFromSeed(0xff6750a4, false) };
const dark: PaintTheme = { scheme: colorSchemeFromSeed(0xff6750a4, true) };
const pressed = { onPressed: { steps: [] } };
const paint = (node: object, r = { x: 0, y: 0, w: 60, h: 40 }, theme = light) => {
  const n = { id: "n", ...node };
  return cupertinoPainterOf(n, theme)?.(n, r, theme, () => undefined);
};

describe("[STU-005] [WGT-011] Cupertino widgets with iOS's colours", () => {
  test("a switch: a 51 by 31 track, system green when on, the white thumb at its end", () => {
    const on = paint({ type: "CupertinoSwitch", props: { value: true } })?.boxes ?? [];
    expect(on[0]).toMatchObject({ x: 4.5, y: 4.5, w: 51, h: 31, fill: 0x34c759ff });
    expect(on[1]).toMatchObject({ x: 26.5, w: 27, oval: true, fill: 0xffffffff });
    expect(paint({ type: "CupertinoSwitch", props: { value: true } }, undefined, dark)?.boxes[0]?.fill).toBe(
      0x30d158ff,
    );
    expect(paint({ type: "CupertinoSwitch", props: { value: false } })?.boxes[1]?.x).toBe(6.5);
  });

  test("a button: plain in primary, filled in its colour with the contrasting label; disabled in grey", () => {
    expect(paint({ type: "CupertinoButton", events: pressed })?.boxes).toEqual([]);
    expect(paint({ type: "CupertinoButton", events: pressed, props: { color: "#007AFF" } })?.boxes[0]).toMatchObject({
      radii: [8, 8, 8, 8],
      fill: 0x007affff,
    });
    expect(cupertinoForeground({ id: "b", type: "CupertinoButton", events: pressed }, light)?.text).toBe(
      argbToRgba(light.scheme.primary),
    );
    expect(cupertinoForeground({ id: "b", type: "CupertinoButton" }, light)?.text).toBe(0x999999ff);
  });

  test("a text field shows its placeholder in the placeholder colour", () => {
    const field = paint(
      { type: "CupertinoTextField", props: { placeholder: "Search" } },
      { x: 0, y: 0, w: 200, h: 36 },
    );
    expect(field?.boxes[0]).toMatchObject({ radii: [5, 5, 5, 5] });
    expect(field?.texts[0]).toMatchObject({ text: "Search", x: 7, color: 0x3c3c434c });
  });

  test("adaptive Material controls are drawn the iOS way under the Cupertino preview only", () => {
    const sw = { type: "Switch", props: { value: true, adaptive: true } };
    expect(paint(sw)).toBeUndefined();
    expect(paint(sw, undefined, { ...light, platform: "cupertino" })?.boxes[0]).toMatchObject({
      w: 51,
      fill: 0x34c759ff,
    });
    const plain = { type: "Switch", props: { value: true } };
    expect(paint(plain, undefined, { ...light, platform: "cupertino" })).toBeUndefined();
  });
});
