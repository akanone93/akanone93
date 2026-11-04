// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { colorSchemeFromSeed } from "../material-color/scheme.ts";
import { checkboxMark, controlForeground, controlPainters, disabledControl, withAlpha } from "./controls.ts";
import { argbToRgba, type PaintTheme } from "./paint.ts";

const scheme = colorSchemeFromSeed(0xff6750a4, false);
const theme: PaintTheme = { scheme };
const role = (r: keyof typeof scheme) => argbToRgba(scheme[r] as number);
const pressed = { onPressed: { steps: [] } };
const paint = (type: string, node: object, r = { x: 0, y: 0, w: 120, h: 48 }) =>
  controlPainters.get(type)?.({ type, ...node }, r, theme) ?? [];

describe("[STU-005] Material 3 controls as Flutter's defaults paint them", () => {
  test("a control with no action is disabled, as Flutter disables a button whose onPressed is null", () => {
    expect(disabledControl({ type: "FilledButton" })).toBe(true);
    expect(disabledControl({ type: "FilledButton", events: pressed })).toBe(false);
    expect(disabledControl({ type: "Switch", events: { onChanged: { steps: [] } } })).toBe(false);
    expect(disabledControl({ type: "Text" })).toBe(false);
  });

  test("buttons: a 40 high pill in their container colour; disabled, onSurface at 12 %", () => {
    expect(paint("FilledButton", { events: pressed })).toEqual([
      { x: 0, y: 4, w: 120, h: 40, radii: [20, 20, 20, 20], fill: role("primary") },
    ]);
    expect(paint("FilledTonalButton", { events: pressed })[0]?.fill).toBe(role("secondaryContainer"));
    expect(paint("ElevatedButton", { events: pressed })[0]?.shadows?.length).toBeGreaterThan(0);
    expect(paint("OutlinedButton", { events: pressed })[0]?.border).toEqual({
      kind: "all",
      side: { color: role("outline"), width: 1 },
    });
    expect(paint("TextButton", { events: pressed })).toEqual([]);
    expect(paint("FilledButton", {})[0]?.fill).toBe(withAlpha(role("onSurface"), 0x1f));
  });

  test("their labels and icons: onPrimary in a filled button, onSurface at 38 % disabled", () => {
    expect(controlForeground({ type: "FilledButton", events: pressed }, theme)).toEqual({
      text: role("onPrimary"),
      icon: role("onPrimary"),
      iconSize: 18,
    });
    expect(controlForeground({ type: "FilledButton" }, theme)?.text).toBe(withAlpha(role("onSurface"), 0x61));
    const selected = { type: "FilterChip", props: { selected: true }, events: { onSelected: { steps: [] } } };
    expect(controlForeground(selected, theme)?.text).toBe(role("onSecondaryContainer"));
  });

  test("chips, FABs, checkboxes, radios, switches, sliders, progress and badges", () => {
    expect(paint("FilterChip", { props: { selected: true } })[0]).toMatchObject({
      h: 32,
      fill: role("secondaryContainer"),
    });
    expect(paint("FloatingActionButton", { events: pressed }, { x: 0, y: 0, w: 56, h: 56 })[0]).toMatchObject({
      w: 56,
      radii: [16, 16, 16, 16],
      fill: role("primaryContainer"),
    });
    const box = { x: 0, y: 0, w: 48, h: 48 };
    const on = { events: { onChanged: { steps: [] } } };
    expect(paint("Checkbox", { ...on, props: { value: true } }, box)).toEqual([
      { x: 15, y: 15, w: 18, h: 18, radii: [2, 2, 2, 2], fill: role("primary") },
    ]);
    expect(checkboxMark({ type: "Checkbox", ...on, props: { value: true } }, theme)).toEqual({
      name: "check",
      color: role("onPrimary"),
    });
    expect(paint("Radio", { ...on, props: { value: 1, groupValue: 1 } }, box)).toHaveLength(2);
    // The 52 pixel track centred in 60; the thumb's centre 16 from its end.
    expect(paint("Switch", { ...on, props: { value: true } }, { x: 0, y: 0, w: 60, h: 48 })[1]).toMatchObject({
      x: 28,
      w: 24,
      fill: role("onPrimary"),
    });
    const slider = paint("Slider", { ...on, props: { value: 0.5 } }, { x: 0, y: 0, w: 240, h: 48 });
    expect(slider[2]).toMatchObject({ x: 110, y: 14, w: 20, oval: true });
    expect(paint("LinearProgressIndicator", { props: { value: 0.25 } }, { x: 0, y: 0, w: 200, h: 4 })[1]?.w).toBe(50);
    expect(paint("Badge", {}, { x: 0, y: 0, w: 24, h: 24 })[0]).toMatchObject({ x: 18, w: 6, fill: role("error") });
  });
});
