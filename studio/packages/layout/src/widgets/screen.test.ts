// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { loadCanvasKit, testParagraphEngine } from "../../test-support/canvaskit.ts";
import type { CanvasKitParagraphEngine } from "../text/canvaskit-engine.ts";
import { defaultScreenTheme, layoutScreen, type ScreenTheme, type WidgetNode } from "./screen.ts";

let engine: CanvasKitParagraphEngine;
const theme: ScreenTheme = { ...defaultScreenTheme, fontFamilies: ["NotoSans", "NotoNaskhArabic"] };
const text = (id: string, data: string): WidgetNode => ({ id, type: "Text", props: { data } });

beforeAll(async () => {
  engine = testParagraphEngine(await loadCanvasKit());
});

afterAll(() => {
  engine.dispose();
});

/** The width and height of `data` as one unconstrained paragraph in `families`. */
function measure(data: string, families: readonly string[]): { w: number; h: number } {
  const layout = layoutScreen(text("t", data), { engine, theme: { ...theme, fontFamilies: families } });
  const box = layout.boxes.find((b) => b.id === "t");
  return { w: box?.w ?? -1, h: box?.h ?? -1 };
}

describe("layoutScreen (STU-005, STU-006)", () => {
  test("lays a column of text and a button out with the engine's text sizes", () => {
    const root: WidgetNode = {
      id: "scaffold",
      type: "Scaffold",
      slots: {
        body: {
          id: "col",
          type: "Column",
          children: [text("title", "Welcome"), { id: "go", type: "FilledButton", slots: { child: text("l", "Go") } }],
        },
      },
    };
    const layout = layoutScreen(root, { engine, theme });
    const byId = new Map(layout.boxes.map((b) => [b.id, b]));
    const title = byId.get("title");
    const go = byId.get("go");
    expect(title).toMatchObject({ kind: "text", text: "Welcome", x: 16, y: 16 });
    // A 16-px Noto Sans line is not the old estimate (7 chars × 8.8 px × 1.4 lines).
    expect(title?.h).toBeCloseTo(measure("Welcome", theme.fontFamilies).h, 5);
    expect(title?.w).toBeGreaterThan(0);
    expect(title?.w).not.toBeCloseTo(7 * 16 * 0.55, 1);
    expect(go).toMatchObject({ kind: "box", text: "Go", h: 48, y: 16 + (title?.h ?? 0) + 8 });
    // A Material 3 button: its label (14 px, w500) between 24-px paddings, at least 64 px wide (S4.3).
    expect(go?.w).toBeGreaterThanOrEqual(64);
    expect(go?.w).toBeCloseTo(66.86, 1);
    expect(byId.has("l")).toBe(false);
    expect(layout.contentHeight).toBeCloseTo(16 + (title?.h ?? 0) + 8 + 48 + 16, 5);
  });

  test("an unknown widget type becomes a labelled placeholder, not an error", () => {
    const root: WidgetNode = {
      id: "col",
      type: "Column",
      children: [{ id: "x", type: "FancyChart", children: [text("inner", "hidden")] }],
    };
    const layout = layoutScreen(root, { engine, theme });
    const x = layout.boxes.find((b) => b.id === "x");
    expect(x).toMatchObject({ kind: "placeholder", text: "FancyChart", h: 48 });
    expect(layout.boxes.some((b) => b.id === "inner")).toBe(false);
  });

  test("a row shares its width, so long text wraps", () => {
    const long = "word ".repeat(40).trim();
    const root: WidgetNode = { id: "row", type: "Row", children: [text("a", long), text("b", "b")] };
    const layout = layoutScreen(root, { engine, theme });
    const a = layout.boxes.find((b) => b.id === "a");
    expect(a?.w).toBeLessThanOrEqual((390 - 32 - 8) / 2);
    expect(a?.h).toBeGreaterThan(measure("word", theme.fontFamilies).h * 3);
  });

  test("an app bar slot adds the bar above the padded body", () => {
    const root: WidgetNode = {
      id: "s",
      type: "Scaffold",
      slots: { appBar: { id: "bar", type: "AppBar" }, body: text("t", "Hi") },
    };
    const layout = layoutScreen(root, { engine, theme, title: "Home" });
    expect(layout.boxes.find((b) => b.id === "appBar")).toMatchObject({
      kind: "box",
      text: "Home",
      y: 0,
      w: 390,
      h: 56,
    });
    expect(layout.boxes.find((b) => b.id === "t")?.y).toBe(56 + 16);
  });

  test("glyphs missing from the first family fall back to the next (mixed script)", () => {
    const arabic = "مرحبا";
    expect(measure(arabic, ["NotoSans", "NotoNaskhArabic"]).w).toBeCloseTo(measure(arabic, ["NotoNaskhArabic"]).w, 5);
    expect(measure(`Hello ${arabic}`, ["NotoSans", "NotoNaskhArabic"]).w).toBeGreaterThan(
      measure("Hello ", ["NotoSans"]).w,
    );
  });

  test("tall content reports its real height beyond the device (STU-056)", () => {
    const root: WidgetNode = {
      id: "col",
      type: "Column",
      children: Array.from({ length: 40 }, (_, i) => text(`t${i}`, `Line ${i}`)),
    };
    const layout = layoutScreen(root, { engine, theme });
    const last = layout.boxes.find((b) => b.id === "t39");
    expect(layout.contentHeight).toBeCloseTo((last?.y ?? 0) + (last?.h ?? 0) + 16, 5);
    expect(layout.contentHeight).toBeGreaterThan(844);
  });

  test("sizes Material buttons, the FAB and list tiles as Flutter does (S4.3)", () => {
    const button = (id: string, type: string, props: Record<string, unknown>): WidgetNode => ({
      id,
      type,
      props: { data: undefined, ...props },
      slots: { child: text(`${id}t`, "Save") },
    });
    const tile = (id: string, props: Record<string, unknown>, parts: readonly string[]): WidgetNode => ({
      id,
      type: "ListTile",
      props,
      slots: Object.fromEntries(parts.map((p) => [p, text(`${id}${p}`, p)])),
    });
    const root: WidgetNode = {
      id: "col",
      type: "Column",
      children: [
        button("padded", "TextButton", {}),
        button("wrap", "OutlinedButton", { tapTargetSize: "shrinkWrap" }),
        button("compact", "ElevatedButton", { density: "compact", tapTargetSize: "shrinkWrap" }),
        button("tonal", "FilledTonalButton", {}),
        button("icon", "IconButton", { tapTargetSize: "shrinkWrap" }),
        { id: "fab", type: "FloatingActionButton", props: {}, slots: { child: text("fabt", "+") } },
        { id: "small", type: "FloatingActionButton", props: { kind: "small" } },
        { id: "unknown", type: "FloatingActionButton", props: { kind: "huge" } },
        { id: "chip", type: "Chip", slots: { label: text("chipt", "Label") } },
        { id: "field", type: "TextField", props: { border: "outline", labelText: "Name" } },
        { id: "helped", type: "TextField", props: { helperText: "Help" } },
        tile("one", {}, ["title"]),
        tile("two", {}, ["title", "subtitle"]),
        tile("lead", { dense: true }, ["title", "leading", "trailing"]),
        tile("three", { isThreeLine: true, density: "compact" }, ["title", "subtitle"]),
        { id: "bare", type: "ListTile", props: {} },
      ],
    };
    const byId = new Map(layoutScreen(root, { engine, theme }).boxes.map((b) => [b.id, b]));
    expect(byId.get("padded")?.h).toBe(48);
    expect(byId.get("wrap")?.h).toBe(40);
    expect(byId.get("compact")?.h).toBe(32);
    expect(byId.get("tonal")?.h).toBe(48);
    expect(byId.get("icon")).toMatchObject({ w: 40, h: 40 });
    expect(byId.get("fab")).toMatchObject({ w: 56, h: 56 });
    expect(byId.get("small")).toMatchObject({ w: 48, h: 48 });
    expect(byId.get("unknown")).toMatchObject({ w: 56, h: 56 });
    expect(byId.get("chip")?.h).toBe(48);
    expect(byId.get("field")?.h).toBe(56);
    expect(byId.get("helped")?.h).toBeGreaterThan(56);
    expect(byId.get("one")?.h).toBe(56);
    expect(byId.get("two")?.h).toBe(72);
    expect(byId.get("lead")?.h).toBe(48);
    // Short lines cannot keep the three-line spec position, so the tile hugs 8 + title + subtitle + 8 ("compat mode").
    expect(byId.get("three")?.h).toBe(60);
    expect(byId.get("bare")?.h).toBe(56);
  });
});
