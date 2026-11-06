// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { colorSchemeFromSeed } from "../material-color/scheme.ts";
import { controlForeground, controlPainters, controlStates, withAlpha } from "./controls.ts";
import { argbToRgba, type PaintTheme } from "./paint.ts";
import { isStateValue, previewedState, resolveState } from "./widget-state.ts";

const scheme = colorSchemeFromSeed(0xff6750a4, false);
const theme: PaintTheme = { scheme };
const role = (r: keyof typeof scheme) => argbToRgba(scheme[r] as number);
const pressed = { onPressed: { steps: [] } };
const paint = (node: object, type = "FilledButton") =>
  controlPainters.get(type)?.({ type, ...node }, { x: 0, y: 0, w: 120, h: 48 }, theme) ?? [];

describe("PC-21 widget states on the canvas [STU-062]", () => {
  test("a state value resolves to its first set state in Flutter's order, else its default", () => {
    const v = { value: "#111111", pressed: "#222222", disabled: "#333333" };
    expect(isStateValue(v)).toBe(true);
    expect(isStateValue({ all: 4 })).toBe(false);
    expect(resolveState(v, new Set())).toBe("#111111");
    expect(resolveState(v, new Set(["pressed"]))).toBe("#222222");
    expect(resolveState(v, new Set(["pressed", "disabled"]))).toBe("#333333");
    expect(resolveState(v, new Set(["hovered"]))).toBe("#111111");
    expect(resolveState({ value: 8 }, new Set(["pressed"]))).toBe(8);
    expect(resolveState({ all: 4 }, new Set(["pressed"]))).toEqual({ all: 4 });
    expect(previewedState({ "x-studio": { state: "hovered" } })).toBe("hovered");
    expect(previewedState({ "x-studio": { state: "nope" } })).toBeUndefined();
  });

  test("a button paints its style: background, shape, outline and content colours for its states", () => {
    const style = {
      backgroundColor: { value: "#00AA00FF", pressed: "#005500FF", disabled: "#CCCCCCFF" },
      foregroundColor: { value: "#FFFFFFFF" },
      shape: { value: { kind: "roundedRectangle", borderRadius: { all: 8 } } },
      side: { value: { color: "#000000FF", width: 2 } },
    };
    const [box] = paint({ events: pressed, props: { style } });
    expect(box).toMatchObject({ fill: 0x00aa00ff, radii: [8, 8, 8, 8], border: { kind: "all" } });
    expect(paint({ props: { style } })[0]?.fill).toBe(0xccccccff);
    expect(controlForeground({ type: "FilledButton", events: pressed, props: { style } }, theme)?.text).toBe(
      0xffffffff,
    );
    expect(paint({ events: pressed })).toEqual([
      { x: 0, y: 4, w: 120, h: 40, radii: [20, 20, 20, 20], fill: role("primary") },
    ]);
  });

  test("a previewed pointer state takes its state's values and Material's state layer over the button", () => {
    const style = { backgroundColor: { value: "#00AA00FF", pressed: "#005500FF" } };
    const node = { events: pressed, props: { style }, "x-studio": { state: "pressed" } };
    expect(controlStates({ type: "FilledButton", ...node })).toEqual(new Set(["pressed"]));
    const [box, layer] = paint(node);
    expect(box?.fill).toBe(0x005500ff);
    expect(layer).toMatchObject({ radii: [20, 20, 20, 20], fill: withAlpha(role("onPrimary"), 26) });
    const text = paint({ events: pressed, "x-studio": { state: "hovered" } }, "TextButton");
    expect(text).toHaveLength(2);
    expect(text[0]?.fill).toBeUndefined();
    expect(text[1]?.fill).toBe(withAlpha(role("primary"), 20));
  });
});
