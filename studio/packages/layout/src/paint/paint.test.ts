// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { colorSchemeFromSeed } from "../material-color/scheme.ts";
import {
  argbToRgba,
  borderRadius,
  decorationPaint,
  edgeInsets,
  type PaintTheme,
  parseHexColor,
  resolveColor,
} from "./paint.ts";
import { type PaintNode, paintTree } from "./painters.ts";

const scheme = colorSchemeFromSeed(0xff6750a4, false);
const theme: PaintTheme = { scheme, tokens: { "color.brand": 0x112233ff } };
const hex = (digits: string) => `#${digits}`;
const rect = { x: 10, y: 20, w: 200, h: 100 };

describe("[STU-005] paint values as the runtime reads them", () => {
  test("colour literals are RRGGBB or RRGGBBAA; tokens resolve to scheme roles or theme tokens", () => {
    expect(parseHexColor(hex("6750A4"))).toBe(0x6750a4ff);
    expect(parseHexColor(hex("F4433633"))).toBe(0xf4433633);
    expect(parseHexColor("red")).toBeUndefined();
    expect(resolveColor({ $token: "color.primary" }, theme)).toBe(argbToRgba(scheme.primary));
    expect(resolveColor({ $token: "color.brand" }, theme)).toBe(0x112233ff);
    expect(resolveColor({ $expr: "state.c" }, theme)).toBeUndefined();
  });

  test("insets swap start and end in a right-to-left screen; radii come per corner or for all", () => {
    expect(edgeInsets({ start: 4, end: 8, vertical: 2 })).toEqual([4, 2, 8, 2]);
    expect(edgeInsets({ start: 4, end: 8, vertical: 2 }, true)).toEqual([8, 2, 4, 2]);
    expect(borderRadius({ all: 12 })).toEqual([12, 12, 12, 12]);
    expect(borderRadius({ topLeft: { circular: 6 } })).toEqual([6, 0, 0, 0]);
    expect(borderRadius({ all: 0 })).toBeUndefined();
  });

  test("a BoxDecoration paints its fill, gradient, border, shadows and shape", () => {
    const paint = decorationPaint(
      {
        color: hex("FFFFFF"),
        gradient: { kind: "linear", colors: [hex("000000"), { $token: "color.primary" }] },
        borderRadius: { all: 8 },
        border: { all: { width: 2, color: hex("FF0000") } },
        boxShadow: [{ color: hex("00000033"), offset: { dx: 0, dy: 4 }, blurRadius: 8 }],
      },
      rect,
      theme,
    );
    expect(paint).toMatchObject({ ...rect, fill: 0xffffffff, radii: [8, 8, 8, 8] });
    expect(paint?.gradient).toMatchObject({ kind: "linear", from: [10, 70], to: [210, 70] });
    expect(paint?.border).toEqual({ kind: "all", side: { color: 0xff0000ff, width: 2 } });
    expect(paint?.shadows).toEqual([{ color: 0x00000033, dx: 0, dy: 4, blur: 8, spread: 0 }]);
    expect(decorationPaint({ shape: "circle", color: hex("FFFFFF") }, rect, theme)?.oval).toBe(true);
    expect(decorationPaint({}, rect, theme)).toBeUndefined();
  });
});

describe("[STU-005] what each widget paints, in paint order", () => {
  const rects = new Map(
    ["scaffold", "col", "card", "text", "box", "clip", "inner", "fade", "faded", "stack", "a", "b"].map((id, i) => [
      id,
      { x: 0, y: i * 10, w: 100, h: 10 },
    ]),
  );
  const tree: PaintNode = {
    id: "scaffold",
    type: "Scaffold",
    slots: {
      body: {
        id: "col",
        type: "Column",
        children: [
          { id: "card", type: "Card", children: [{ id: "text", type: "Text", props: { data: "Hi" } }] },
          { id: "box", type: "Container", props: { margin: { all: 2 }, color: { $token: "color.tertiary" } } },
          {
            id: "clip",
            type: "ClipRRect",
            props: { borderRadius: { all: 4 } },
            children: [{ id: "inner", type: "ColoredBox", props: { color: hex("00FF00") } }],
          },
          { id: "fade", type: "Opacity", props: { opacity: 0.5 }, children: [{ id: "faded", type: "ElevatedButton" }] },
          {
            id: "stack",
            type: "IndexedStack",
            props: { index: 1 },
            children: [
              { id: "a", type: "Text", props: { data: "first" } },
              { id: "b", type: "Text", props: { data: "second" } },
            ],
          },
        ],
      },
    },
  };
  const paints = paintTree(tree, rects, theme);

  test("the scaffold paints the surface; layout widgets paint nothing; texts take onSurface", () => {
    expect(paints.get("scaffold")?.boxes[0]?.fill).toBe(argbToRgba(scheme.surface));
    expect(paints.get("col")?.role).toBe("layout");
    expect(paints.get("text")).toMatchObject({ role: "text", textColor: argbToRgba(scheme.onSurface) });
  });

  test("a card is Material 3's: inside a 4-pixel margin, 12-pixel corners, surfaceContainerLow, elevation 1", () => {
    const box = paints.get("card")?.boxes[0];
    expect(box).toMatchObject({ x: 4, y: 24, w: 92, h: 2, radii: [12, 12, 12, 12] });
    expect(box?.fill).toBe(argbToRgba(scheme.surfaceContainerLow));
    expect(box?.shadows).toHaveLength(3);
  });

  test("a container paints its colour inside its margin", () => {
    expect(paints.get("box")?.boxes[0]).toMatchObject({ x: 2, y: 42, w: 96, h: 6, fill: argbToRgba(scheme.tertiary) });
  });

  test("clips and opacity reach the descendants; an unpainted widget is a placeholder", () => {
    expect(paints.get("inner")?.clips).toEqual([{ x: 0, y: 50, w: 100, h: 10, radii: [4, 4, 4, 4] }]);
    expect(paints.get("faded")).toMatchObject({ role: "placeholder", opacity: 0.5 });
  });

  test("an IndexedStack paints only the child at its index", () => {
    expect(paints.get("a")?.role).toBe("hidden");
    expect(paints.get("b")?.role).toBe("text");
  });

  test("a later widget is painted over an earlier one", () => {
    const order = (id: string) => paints.get(id)?.order ?? -1;
    expect(order("scaffold")).toBeLessThan(order("card"));
    expect(order("card")).toBeLessThan(order("text"));
    expect(order("text")).toBeLessThan(order("box"));
  });
});

describe("[THM-005] icons as Flutter's Icon draws them", () => {
  const rects = (ids: string[]) => new Map(ids.map((id) => [id, { x: 0, y: 0, w: 48, h: 48 }]));
  const icon = (id: string, props: Record<string, unknown> = {}): PaintNode => ({
    id,
    type: "Icon",
    props: { icon: { name: "home" }, ...props },
  });

  test("an icon on its own: 24 pixels in Flutter's default icon colour, black at 87 % or white in dark", () => {
    const light = paintTree(icon("i"), rects(["i"]), theme).get("i");
    expect(light).toMatchObject({ role: "icon", icon: { name: "home", set: "material", size: 24 } });
    expect(light?.icon?.color).toBe(argbToRgba(0xdd000000));
    const dark = paintTree(icon("i"), rects(["i"]), { scheme: colorSchemeFromSeed(0xff6750a4, true) }).get("i");
    expect(dark?.icon?.color).toBe(0xffffffff);
  });

  test("its size and colour, else its component's icon theme; an icon without a name is a placeholder", () => {
    const root: PaintNode = {
      id: "col",
      type: "Column",
      children: [
        icon("own", { size: 32, color: { $token: "color.primary" }, icon: { name: "heart", set: "cupertino" } }),
        { id: "b", type: "FilledButton", slots: { child: icon("inFilled") } },
        { id: "bar", type: "AppBar", slots: { leading: icon("lead"), actions: [icon("act")] } },
        icon("none", { icon: { $expr: "page.icon" } }),
      ],
    };
    const paints = paintTree(root, rects(["col", "own", "b", "inFilled", "bar", "lead", "act", "none"]), theme);
    expect(paints.get("own")?.icon).toEqual({
      name: "heart",
      set: "cupertino",
      size: 32,
      color: argbToRgba(scheme.primary),
    });
    expect(paints.get("inFilled")?.icon).toMatchObject({ size: 18, color: argbToRgba(scheme.onPrimary) });
    expect(paints.get("lead")?.icon?.color).toBe(argbToRgba(scheme.onSurface));
    expect(paints.get("act")?.icon?.color).toBe(argbToRgba(scheme.onSurfaceVariant));
    expect(paints.get("none")?.role).toBe("placeholder");
  });
});
