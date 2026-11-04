// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { colorSchemeFromSeed } from "../material-color/scheme.ts";
import { barPainters, barSurface } from "./bars.ts";
import { argbToRgba, type PaintTheme } from "./paint.ts";

const scheme = colorSchemeFromSeed(0xff6750a4, false);
const theme: PaintTheme = { scheme };
const role = (r: keyof typeof scheme) => argbToRgba(scheme[r] as number);

describe("[STU-005] Material 3 bars as Flutter's defaults paint them", () => {
  test("a navigation bar: surfaceContainer, the selected destination's pill and colours", () => {
    const rects: Record<string, { x: number; y: number; w: number; h: number }> = {
      "n.0.icon": { x: 40, y: 16, w: 24, h: 24 },
      "n.1.icon": { x: 160, y: 16, w: 24, h: 24 },
      "n.0#label": { x: 30, y: 52, w: 40, h: 16 },
      "n.1#label": { x: 150, y: 52, w: 40, h: 16 },
    };
    const node = {
      id: "n",
      type: "NavigationBar",
      props: {
        selectedIndex: 1,
        destinations: [
          { label: "Home", icon: { name: "home" } },
          { label: "Pay", icon: { name: "payments" } },
        ],
      },
    };
    const paint = barPainters.get("NavigationBar")?.(node, { x: 0, y: 0, w: 240, h: 80 }, theme, (id) => rects[id]);
    expect(paint?.boxes[0]?.fill).toBe(role("surfaceContainer"));
    expect(paint?.boxes[1]).toMatchObject({ x: 140, y: 12, w: 64, h: 32, fill: role("secondaryContainer") });
    expect(paint?.glyphs.map((g) => [g.name, g.color])).toEqual([
      ["home", role("onSurfaceVariant")],
      ["payments", role("onSecondaryContainer")],
    ]);
    expect(paint?.texts.map((t) => [t.text, t.color])).toEqual([
      ["Home", role("onSurfaceVariant")],
      ["Pay", role("onSurface")],
    ]);
  });

  test("a tab bar: a divider, the selected tab's label in primary over a 3 pixel indicator", () => {
    const rects: Record<string, { x: number; y: number; w: number; h: number }> = {
      "t.0#label": { x: 20, y: 14, w: 60, h: 20 },
      "t.1#label": { x: 140, y: 14, w: 50, h: 20 },
    };
    const node = { id: "t", type: "TabBar", props: { tabs: [{ label: "One" }, { label: "Two" }] } };
    const paint = barPainters.get("TabBar")?.(node, { x: 0, y: 0, w: 240, h: 48 }, theme, (id) => rects[id]);
    expect(paint?.boxes).toEqual([
      { x: 0, y: 47, w: 240, h: 1, fill: role("outlineVariant") },
      { x: 20, y: 45, w: 60, h: 3, radii: [3, 3, 0, 0], fill: role("primary") },
    ]);
    expect(paint?.texts.map((t) => t.color)).toEqual([role("primary"), role("onSurfaceVariant")]);
  });

  test("app bars, bottom app bars and drawers fill their surfaces; a background colour wins", () => {
    const r = { x: 0, y: 0, w: 100, h: 56 };
    expect(barSurface({ id: "a", type: "AppBar" }, r, theme)?.[0]?.fill).toBe(role("surface"));
    expect(barSurface({ id: "b", type: "BottomAppBar" }, r, theme)?.[0]?.fill).toBe(role("surfaceContainer"));
    expect(barSurface({ id: "d", type: "Drawer" }, r, theme)?.[0]).toMatchObject({ radii: [0, 16, 16, 0] });
    expect(barSurface({ id: "a", type: "AppBar", props: { backgroundColor: "#FF0000" } }, r, theme)?.[0]?.fill).toBe(
      0xff0000ff,
    );
    expect(barSurface({ id: "x", type: "Text" }, r, theme)).toBeUndefined();
  });
});
