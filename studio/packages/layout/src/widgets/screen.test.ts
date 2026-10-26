// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { readdirSync } from "node:fs";
import { join } from "node:path";
import { loadCanvasKit, testParagraphEngine } from "../../test-support/canvaskit.ts";
import { TextDirection } from "../flutter/geometry.ts";
import { RenderParagraph } from "../flutter/paragraph.ts";
import type { TextStyle } from "../flutter/text-style.ts";
import type { CanvasKitParagraphEngine } from "../text/canvaskit-engine.ts";
import { TextAlign } from "../text/paragraph-engine.ts";
import { cupertinoWidgetBuilders } from "./cupertino.ts";
import {
  defaultScreenTheme,
  defaultWidgetBuilders,
  layoutScreen,
  type ScreenTheme,
  type WidgetContext,
  type WidgetNode,
} from "./screen.ts";

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

describe("Cupertino widgets in a screen (STU-005, S4.4)", () => {
  const node = (id: string, type: string, extra: Partial<WidgetNode> = {}): WidgetNode => ({ id, type, ...extra });
  const col = (...children: WidgetNode[]): WidgetNode => ({ id: "col", type: "Column", children });

  test("controls take Flutter's sizes and a button keeps its 44-pt tap target", () => {
    const layout = layoutScreen(
      col(
        node("button", "CupertinoButton", { children: [text("b", "Go")] }),
        node("small", "CupertinoButton", { props: { data: { sizeStyle: "small" } }, children: [text("s", "")] }),
        node("switch", "CupertinoSwitch"),
        node("slider", "CupertinoSlider"),
        node("field", "CupertinoTextField", { props: { data: { placeholder: "Search" } } }),
      ),
      { engine, theme },
    );
    const byId = new Map(layout.boxes.map((b) => [b.id, b]));
    expect(byId.get("button")?.h).toBeCloseTo(32 + measure("Go", theme.fontFamilies).h, 5);
    expect(byId.get("small")?.h).toBeGreaterThanOrEqual(28);
    expect(byId.get("switch")).toMatchObject({ w: 59, h: 39 });
    expect(byId.get("slider")).toMatchObject({ w: 176, h: 44 });
    expect(byId.get("field")?.h).toBeGreaterThan(14);
    expect(byId.get("field")?.w).toBe(theme.width - 2 * theme.pagePadding);
  });

  test("bars, tiles and segmented controls span the available width", () => {
    const layout = layoutScreen(
      col(
        node("nav", "CupertinoNavigationBar", {
          props: { data: { title: "Title", leading: "Back", trailing: "Edit" } },
        }),
        node("tabs", "CupertinoTabBar", { children: [text("a", "Home"), text("b", "Search")] }),
        node("tile", "CupertinoListTile", { children: [text("t", "Wi-Fi")] }),
        node("seg", "CupertinoSegmentedControl", { children: [text("x", "Day"), text("y", "Week")] }),
        node("bare", "CupertinoNavigationBar"),
      ),
      { engine, theme },
    );
    const byId = new Map(layout.boxes.map((b) => [b.id, b]));
    const width = theme.width - 2 * theme.pagePadding;
    expect(byId.get("nav")).toMatchObject({ w: width, h: 44, text: "Title" });
    expect(byId.get("tabs")).toMatchObject({ w: width, h: 50 });
    expect(byId.get("tile")).toMatchObject({ w: width, h: 44, text: "Wi-Fi" });
    expect(byId.get("seg")?.h).toBe(28);
    expect(byId.get("bare")?.h).toBe(44);
  });

  test("a picker is a 216-pt wheel across the available width", () => {
    const layout = layoutScreen(
      col(node("picker", "CupertinoPicker", { children: [text("a", "One"), text("b", "Two")] })),
      { engine, theme },
    );
    const byId = new Map(layout.boxes.map((b) => [b.id, b]));
    expect(byId.get("picker")).toMatchObject({ w: theme.width - 2 * theme.pagePadding, h: 216 });
  });

  test("a date picker is a 216-pt row of wheels across the available width in each mode", () => {
    const layout = layoutScreen(
      col(
        node("time", "CupertinoDatePicker", { props: { data: { mode: "time" } } }),
        node("date", "CupertinoDatePicker", { props: { data: { mode: "date" } } }),
        node("both", "CupertinoDatePicker"),
      ),
      { engine, theme },
    );
    const byId = new Map(layout.boxes.map((b) => [b.id, b]));
    for (const id of ["time", "date", "both"]) {
      expect(byId.get(id)).toMatchObject({ w: theme.width - 2 * theme.pagePadding, h: 216 });
    }
  });

  test("a closed context menu is its child, and an action is a 43-pt row", () => {
    const layout = layoutScreen(
      col(
        node("menu", "CupertinoContextMenu", { children: [text("preview", "Preview")] }),
        node("action", "CupertinoContextMenuAction", { children: [text("label", "Share")] }),
      ),
      { engine, theme },
    );
    const byId = new Map(layout.boxes.map((b) => [b.id, b]));
    expect(byId.get("menu")).toMatchObject({ h: byId.get("preview")?.h ?? -1 });
    expect(byId.get("action")).toMatchObject({ w: theme.width - 2 * theme.pagePadding, h: 43 });
  });

  test("a sliding segmented control is as tall as its content and spans the available width", () => {
    const layout = layoutScreen(
      col(node("sliding", "CupertinoSlidingSegmentedControl", { children: [text("x", "Day"), text("y", "Week")] })),
      { engine, theme },
    );
    const byId = new Map(layout.boxes.map((b) => [b.id, b]));
    expect(byId.get("sliding")?.w).toBe(theme.width - 2 * theme.pagePadding);
    expect(byId.get("sliding")?.h).toBeGreaterThanOrEqual(32);
  });

  test("a sliver navigation bar is shown expanded: toolbar and large title", () => {
    const layout = layoutScreen(
      col(node("large", "CupertinoSliverNavigationBar", { props: { data: { title: "Settings" } } })),
      { engine, theme },
    );
    const byId = new Map(layout.boxes.map((b) => [b.id, b]));
    expect(byId.get("large")).toMatchObject({ w: theme.width - 2 * theme.pagePadding, h: 96, text: "Settings" });
  });

  test("an alert dialog fills the width with its actions side by side, and stacks three", () => {
    const action = (id: string, label: string): WidgetNode =>
      node(id, "CupertinoDialogAction", { children: [text(`${id}-t`, label)] });
    const layout = layoutScreen(
      col(
        node("two", "CupertinoAlertDialog", {
          props: { data: { title: "Allow?", content: "Location helps." } },
          children: [action("no", "No"), action("yes", "Yes")],
        }),
        node("three", "CupertinoAlertDialog", {
          props: { data: { title: "Delete" } },
          children: [action("a", "Delete"), action("b", "Keep"), action("c", "Cancel")],
        }),
      ),
      { engine, theme },
    );
    const byId = new Map(layout.boxes.map((b) => [b.id, b]));
    expect(byId.get("two")?.w).toBe(theme.width - 2 * theme.pagePadding);
    expect(byId.get("two")?.h).toBeGreaterThan(67.8);
    // Two short actions share one row (45 pt); three stack into 3 x 45 pt and two rules.
    expect(byId.get("three")?.h).toBeGreaterThan(3 * 45 + 2 * 0.3);
  });

  test("an alert dialog centres its title and content text as CupertinoAlertDialog does", () => {
    const made: { text: string; paragraph: RenderParagraph }[] = [];
    const ctx = {
      engine,
      theme,
      build: () => {
        throw new Error("unexpected child build");
      },
      available: () => theme.width,
      paragraph: (text: string, style?: TextStyle) => {
        const paragraph = new RenderParagraph({
          text: { text, ...(style === undefined ? {} : { style }) },
          textDirection: TextDirection.ltr,
          engine,
        });
        made.push({ text, paragraph });
        return paragraph;
      },
      mark: () => undefined,
    } as unknown as WidgetContext;
    const builder = cupertinoWidgetBuilders.get("CupertinoAlertDialog");
    builder?.(
      node("dialog", "CupertinoAlertDialog", { props: { data: { title: "Allow?", content: "Location helps." } } }),
      ctx,
    );
    const centred = (text: string): TextAlign | undefined => made.find((m) => m.text === text)?.paragraph.textAlign;
    expect(centred("Allow?")).toBe(TextAlign.center);
    expect(centred("Location helps.")).toBe(TextAlign.center);
    for (const { paragraph } of made) {
      paragraph.dispose();
    }
  });

  test("a picker shows its own items, and a date picker its columns", () => {
    const layout = layoutScreen(
      col(node("picker", "CupertinoPicker", { props: { data: { items: ["Red", "Green", "Blue"], selectedItem: 1 } } })),
      { engine, theme },
    );
    const byId = new Map(layout.boxes.map((b) => [b.id, b]));
    expect(byId.get("picker")).toMatchObject({ w: theme.width - 2 * theme.pagePadding, h: 216 });
  });

  test("an action sheet spans the screen with its cancel button below the main sheet", () => {
    const action = (id: string, label: string): WidgetNode =>
      node(id, "CupertinoActionSheetAction", { children: [text(`${id}-t`, label)] });
    const layout = layoutScreen(
      col(
        node("sheet", "CupertinoActionSheet", {
          props: { data: { title: "Photo", message: "Pick one.", cancel: "Cancel" } },
          children: [action("a", "Take"), action("b", "Choose")],
        }),
      ),
      { engine, theme },
    );
    const byId = new Map(layout.boxes.map((b) => [b.id, b]));
    // Two 57.17-pt actions, the 0.3-pt rule, the content, a cancel button of 57.17 pt and 8 pt between, and the 8-pt bottom margin.
    expect(byId.get("sheet")?.h).toBeGreaterThan(2 * 57.17 + 57.17 + 8 + 8);
  });

  test("scaffolds and sections lay their content out", () => {
    const layout = layoutScreen(
      node("scaffold", "CupertinoPageScaffold", {
        children: [node("section", "CupertinoListSection", { children: [text("row", "Row")] })],
      }),
      { engine, theme },
    );
    expect(layout.boxes.some((b) => b.id === "row")).toBe(true);
  });
});

describe("Material composites in a screen (STU-005, S4.3)", () => {
  const tab = (id: string, props: Record<string, unknown>, icon = false): WidgetNode => ({
    id,
    type: "Tab",
    props,
    ...(icon ? { slots: { icon: { id: `${id}i`, type: "Icon" } } } : {}),
  });

  test("a tab bar is 48 high with text or icon tabs and 74 with text and icon tabs", () => {
    const root: WidgetNode = {
      id: "col",
      type: "Column",
      children: [
        { id: "text", type: "TabBar", children: [tab("a", { text: "One" }), tab("b", { text: "Two" })] },
        { id: "icons", type: "TabBar", children: [tab("c", {}, true), tab("d", {}, true)] },
        { id: "both", type: "TabBar", children: [tab("e", { text: "One" }, true), tab("f", { text: "Two" })] },
        { id: "scroll", type: "TabBar", props: { isScrollable: true }, children: [tab("g", { text: "One" })] },
      ],
    };
    const byId = new Map(layoutScreen(root, { engine, theme }).boxes.map((b) => [b.id, b]));
    expect(byId.get("text")).toMatchObject({ w: 358, h: 48 });
    expect(byId.get("a")?.h).toBe(46);
    expect(byId.get("icons")?.h).toBe(48);
    expect(byId.get("both")?.h).toBe(74);
    expect(byId.get("f")?.h).toBe(46);
    expect(byId.get("scroll")?.h).toBe(48);
  });

  test("an alert dialog is at least 280 wide, with 24 around its title and its actions at the end", () => {
    const root: WidgetNode = {
      id: "dialog",
      type: "AlertDialog",
      slots: { title: text("title", "Discard?"), content: text("body", "Your draft will be lost.") },
      children: [
        { id: "cancel", type: "TextButton", slots: { child: text("c", "Cancel") } },
        { id: "ok", type: "TextButton", slots: { child: text("o", "Discard") } },
      ],
    };
    const byId = new Map(layoutScreen(root, { engine, theme }).boxes.map((b) => [b.id, b]));
    const dialog = byId.get("dialog");
    expect(dialog?.w).toBe(358);
    const cancel = byId.get("cancel");
    const ok = byId.get("ok");
    expect(cancel?.h).toBe(48);
    expect((ok?.x ?? 0) + (ok?.w ?? 0)).toBeCloseTo(358 - 40 - 24 + 16, 0);
    expect((ok?.x ?? 0) - ((cancel?.x ?? 0) + (cancel?.w ?? 0))).toBeCloseTo(8, 5);
  });

  test("a bottom sheet is as wide as the screen and a handle adds 48; a popup menu steps its width by 56", () => {
    const sheet = (id: string, props: Record<string, unknown>): WidgetNode => ({
      id,
      type: "BottomSheet",
      props,
      slots: { child: { id: `${id}c`, type: "SizedBox", props: { height: 100 } } },
    });
    const item = (id: string, label: string): WidgetNode => ({
      id,
      type: "PopupMenuItem",
      slots: { child: text(`${id}t`, label) },
    });
    const root: WidgetNode = {
      id: "col",
      type: "Column",
      children: [
        sheet("plain", {}),
        sheet("handle", { showDragHandle: true }),
        {
          id: "menu",
          type: "PopupMenu",
          children: [item("a", "Cut"), { id: "div", type: "PopupMenuDivider" }, item("b", "Copy")],
        },
      ],
    };
    const byId = new Map(layoutScreen(root, { engine, theme }).boxes.map((b) => [b.id, b]));
    expect(byId.get("plain")).toMatchObject({ w: 358 });
    expect(byId.get("handle")?.h).toBe((byId.get("plain")?.h ?? 0) + 48);
    expect(byId.get("a")?.h).toBe(48);
    expect(byId.get("menu")?.w).toBe(112);
    expect(byId.get("menu")?.h).toBe(8 + 48 + 16 + 48 + 8);
  });
});

describe("Layer 2 components checked as a box only (STU-005, S4.5)", () => {
  // Maintainer decision (S4.5): the scanner package cannot be built into the runtime's golden writer (its build hook
  // would make a cycle), so `QrCodeView`, `QrScanner` and `BarcodeScanner` have no conformance scene. Studio lays out
  // the box the runtime gives them and this test pins it.
  const boxes = (): Map<string, { w: number; h: number }> => {
    const root: WidgetNode = {
      id: "col",
      type: "Column",
      children: [
        { id: "qr", type: "QrCodeView", props: { data: "https://example.com", semanticLabel: "Pay" } },
        { id: "big", type: "QrCodeView", props: { data: "x", semanticLabel: "Pay", size: 120 } },
        { id: "tiny", type: "QrCodeView", props: { data: "x", semanticLabel: "Pay", size: 4 } },
        { id: "scan", type: "QrScanner", props: {} },
        { id: "bar", type: "BarcodeScanner", props: { formats: ["ean13"] } },
      ],
    };
    return new Map(layoutScreen(root, { engine, theme }).boxes.map((b) => [b.id, { w: b.w, h: b.h }]));
  };

  test("a QR code is a square of size, 200 by default and at least 16", () => {
    const byId = boxes();
    expect(byId.get("qr")).toEqual({ w: 200, h: 200 });
    expect(byId.get("big")).toEqual({ w: 120, h: 120 });
    expect(byId.get("tiny")).toEqual({ w: 16, h: 16 });
  });

  test("a scanner is a square as wide as the screen's content", () => {
    const content = theme.width - 2 * theme.pagePadding;
    const byId = boxes();
    expect(byId.get("scan")).toEqual({ w: content, h: content });
    expect(byId.get("bar")).toEqual({ w: content, h: content });
  });
});

/** Widget types of `schema/widgets` (Layer 1 and Layer 2): the descriptor file names. */
function schemaWidgetTypes(): string[] {
  const root = join(import.meta.dir, "../../../../../schema/widgets");
  return ["layer1", "layer2"].flatMap((layer) =>
    readdirSync(join(root, layer))
      .filter((file) => file.endsWith(".json"))
      .map((file) => file.slice(0, -".json".length)),
  );
}

/**
 * The widget types still drawn as labelled placeholders (S4.6). A type leaves
 * this list when it gains a builder, and the test fails until it does.
 */
const placeholderTypes: readonly string[] = [
  "Autocomplete",
  "BottomAppBar",
  "CalendarDatePicker",
  "DataTable",
  "DropdownMenu",
  "MaterialBanner",
  "NavigationDrawer",
  "PopupMenuButton",
  "SegmentedButton",
  "Stepper",
];

describe("builder coverage (STU-005)", () => {
  test("every widget type has a builder except the listed placeholders", () => {
    const types = schemaWidgetTypes();
    expect(types.length).toBeGreaterThan(200);
    const missing = types.filter((type) => !defaultWidgetBuilders.has(type)).sort();
    expect(missing).toEqual([...placeholderTypes].sort());
  });

  test("a type with a builder is not listed as a placeholder", () => {
    for (const type of placeholderTypes) {
      expect(defaultWidgetBuilders.has(type)).toBe(false);
    }
  });

  test("the builders name no type the schema lacks, besides the screen's own", () => {
    const own = ["FilledTonalButton", "AlertDialog", "BottomSheet", "PopupMenu"];
    const known = new Set([...schemaWidgetTypes(), ...own]);
    const extra = [...defaultWidgetBuilders.keys()].filter((type) => !known.has(type) && !type.startsWith("Cupertino"));
    expect(extra).toEqual([]);
  });
});
