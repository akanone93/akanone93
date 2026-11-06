// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { colorSchemeFromSeed, type PaintTheme } from "@plux/layout";
import {
  addTokenRecipe,
  hexOf,
  matchingToken,
  newTokenName,
  rgbaOf,
  setStyleRecipe,
  setTokenRecipe,
  styleValuesOf,
  tokenUsers,
} from "./theme-round-trip.ts";

const H = "#";
const theme: PaintTheme = {
  scheme: colorSchemeFromSeed(0xff6750a4, false),
  tokens: { "color.primary": 0x6750a4ff, "color.surface": 0xfffbfeff },
  dimensions: { "space.md": 12, "radius.md": 12, "space.lg": 24 },
};

describe("PC-35 the theme round trip [THM-001]", () => {
  test("colours read and write as the documents write them", () => {
    expect(rgbaOf(`${H}6750A4`)).toBe(0x6750a4ff);
    expect(rgbaOf(`${H}6750a480`)).toBe(0x6750a480);
    expect(rgbaOf("red")).toBeUndefined();
    expect(hexOf(0x6750a4ff)).toBe(`${H}6750A4FF`);
  });

  test("a widget's colours and sizes, bound or raw, with the family a size binds to", () => {
    const node = {
      id: "c",
      type: "Container",
      props: {
        color: { $token: "color.primary" },
        padding: { all: 18 },
        decoration: { borderRadius: { all: { $token: "radius.md" } }, color: `${H}FF5722FF` },
        width: 120,
        text: { $expr: "page.width" },
      },
    };
    expect(styleValuesOf(node)).toEqual([
      { path: ["color"], kind: "color", token: "color.primary" },
      { path: ["padding", "all"], kind: "dimension", raw: 18, family: "space" },
      { path: ["decoration", "borderRadius", "all"], kind: "dimension", token: "radius.md", family: "radius" },
      { path: ["decoration", "color"], kind: "color", raw: 0xff5722ff },
      { path: ["width"], kind: "dimension", raw: 120, family: "size" },
    ]);
  });

  test("where a token is used, and the theme's token of a raw value, its family first", () => {
    const roots = [
      { id: "a", type: "Text", props: { style: { color: { $token: "color.primary" } } } },
      { id: "b", type: "Column", children: [{ id: "c", type: "Icon", props: { color: { $token: "color.primary" } } }] },
    ];
    expect(tokenUsers(roots, "color.primary")).toEqual(["a", "c"]);
    expect(tokenUsers(roots, "color.surface")).toEqual([]);
    expect(matchingToken({ path: ["c"], kind: "color", raw: 0x6750a4ff }, theme)).toBe("color.primary");
    expect(matchingToken({ path: ["r"], kind: "dimension", raw: 12, family: "radius" }, theme)).toBe("radius.md");
    expect(matchingToken({ path: ["p"], kind: "dimension", raw: 12, family: "space" }, theme)).toBe("space.md");
    expect(matchingToken({ path: ["p"], kind: "dimension", raw: 7, family: "space" }, theme)).toBeUndefined();
    expect(matchingToken({ path: ["w"], kind: "dimension", raw: 24, family: "size" }, theme)).toBeUndefined();
  });

  test("a new token's name: the family and pixels for a size, a numbered custom colour", () => {
    const taken = new Set(["space.18", "color.custom1"]);
    expect(newTokenName({ path: [], kind: "dimension", raw: 18, family: "space" }, taken)).toBe("space.181");
    expect(newTokenName({ path: [], kind: "dimension", raw: 9.5, family: "radius" }, taken)).toBe("radius.9_5");
    expect(newTokenName({ path: [], kind: "color", raw: 1 }, taken)).toBe("color.custom2");
  });

  test("binding a prop, adding a token and setting a token's value", () => {
    const root = {
      id: "r",
      type: "Column",
      children: [{ id: "x", type: "Container", props: { color: `${H}FF5722FF` } }],
    };
    setStyleRecipe("x", ["decoration", "color"], { $token: "color.accent" })(root);
    expect((root.children[0] as { props: unknown }).props).toEqual({
      color: `${H}FF5722FF`,
      decoration: { color: { $token: "color.accent" } },
    });
    const tokens: Record<string, unknown> = {
      color: { primary: { $value: `${H}6750A4`, $extensions: { "dev.plux.dark": `${H}D0BCFF` } } },
    };
    addTokenRecipe("color.accent", { path: [], kind: "color", raw: 0xff5722ff })(tokens);
    addTokenRecipe("space.18", { path: [], kind: "dimension", raw: 18, family: "space" })(tokens);
    expect(tokens).toMatchObject({
      color: { accent: { $value: `${H}FF5722FF`, $type: "color" } },
      space: { "18": { $value: { unit: "px", value: 18 }, $type: "dimension" } },
    });
    setTokenRecipe("color.primary", "color", 0x112233ff, true)(tokens);
    setTokenRecipe("space.18", "dimension", 20, true)(tokens);
    expect(tokens).toMatchObject({
      color: { primary: { $value: `${H}6750A4`, $extensions: { "dev.plux.dark": `${H}112233FF` } } },
      space: { "18": { $value: { unit: "px", value: 20 } } },
    });
    setTokenRecipe("color.accent", "color", 0x000000ff, true)(tokens);
    expect((tokens["color"] as Record<string, { $value: string }>)["accent"]?.$value).toBe(`${H}000000FF`);
  });
});
