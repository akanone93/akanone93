// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import { colorSchemeFromSeed, defaultScreenTheme, type WidgetNode } from "@plux/layout";
import type { CanvasKit } from "canvaskit-wasm";
import { fontBytes, loadCanvasKit } from "../test-support/canvaskit.ts";
import type { DeviceChromeSpec, ItemSpec, ScreenStyle } from "./protocol.ts";
import { layoutScreens } from "./screens.ts";
import { canvasParagraphEngine } from "./worker.ts";

let ck: CanvasKit;
beforeAll(async () => {
  ck = await loadCanvasKit();
});

const chrome: DeviceChromeSpec = {
  top: 47,
  right: 0,
  bottom: 34,
  left: 0,
  cutout: "notch",
  indicator: "home",
  platform: "material",
  radius: 47,
  landscape: false,
  time: "9:41",
  visible: true,
  onLight: 0x000000e6,
  onDark: 0xffffffff,
  cutoutColor: 0x000000ff,
};

const scheme = colorSchemeFromSeed(0xff6750a4, false);
const style: ScreenStyle = {
  deviceHeight: 844,
  theme: { ...defaultScreenTheme, fontFamilies: ["Noto Sans"] },
  frame: 0xffffffff,
  frameStroke: 0x11,
  box: 0x22,
  boxStroke: 0x33,
  placeholder: 0x44,
  text: 0x55,
  overflowFill: 0x66,
  overflowBox: 0x77,
  overflowText: 0x88,
  overflowStroke: 0x99,
  paint: { scheme },
  chrome,
};

const text = (id: string, data: string): WidgetNode => ({ id, type: "Text", props: { data } });
type Box = { x: number; y: number; w: number; h: number };

function lay(root: WidgetNode): { items: Map<string, ItemSpec & Box>; height: number } {
  const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
  try {
    const out = layoutScreens([{ id: "s", title: "S", x: 0, y: 0, root }], style, engine);
    const items = new Map(
      out.ops.flatMap((op) => (op.op === "add" ? [[op.item.id, op.item as ItemSpec & Box] as const] : [])),
    );
    return { items, height: out.heights[0]?.contentHeight ?? 0 };
  } finally {
    engine.dispose();
  }
}

const nav: WidgetNode = {
  id: "nav",
  type: "NavigationBar",
  props: {
    selectedIndex: 0,
    destinations: [
      { label: "Home", icon: { name: "home" } },
      { label: "Pay", icon: { name: "payments" } },
    ],
  },
};
const fab: WidgetNode = {
  id: "fab",
  type: "FloatingActionButton",
  slots: { child: { id: "fi", type: "Icon", props: { icon: { name: "add" } } } },
};
const rows = (n: number): WidgetNode[] =>
  Array.from({ length: n }, (_, i) => ({ id: `r${i}`, type: "ListTile", slots: { title: text(`t${i}`, `Row ${i}`) } }));

describe("PC-12 a page's scaffold laid out in place [STU-005]", () => {
  test("the navigation bar stands at the bottom over the home indicator, the button above it at the end", () => {
    const { items } = lay({
      id: "sc",
      type: "Scaffold",
      slots: {
        appBar: { id: "bar", type: "AppBar", slots: { title: text("title", "Home") } },
        body: { id: "body", type: "Center", slots: { child: text("hi", "Hello") } },
        bottomNavigationBar: nav,
        floatingActionButton: fab,
      },
    });
    // 80 for the bar and 34 for the home indicator under it, its surface reaching the bottom of the device.
    expect(items.get("s/nav#paint0")).toMatchObject({ y: 844 - 114, h: 114 });
    const button = items.get("s/fab#paint0");
    expect(button).toMatchObject({ w: 56, h: 56 });
    expect(button?.x).toBe(390 - 16 - 56);
    expect(button?.y).toBe(844 - 114 - 16 - 56);
    // The body is between the app bar (56 under a 47 status bar) and the navigation bar: its centre is there.
    const hi = items.get("s/hi") as Box;
    const centre = 103 + (844 - 114 - 103) / 2;
    expect(hi.y).toBeGreaterThan(centre - 20);
    expect(hi.y).toBeLessThan(centre);
  });

  test("without a bottom bar the button keeps clear of the home indicator", () => {
    const { items } = lay({
      id: "sc",
      type: "Scaffold",
      slots: { body: text("b", "Body"), floatingActionButton: fab },
    });
    expect(items.get("s/fab#paint0")?.y).toBe(844 - 34 - 16 - 56);
  });

  test("a scrolling body keeps its whole content, which runs past the frame into the overflow region", () => {
    const { items, height } = lay({
      id: "sc",
      type: "Scaffold",
      slots: { body: { id: "list", type: "ListView", children: rows(30) }, bottomNavigationBar: nav },
    });
    expect(height).toBeGreaterThan(844 + 400);
    expect(items.has("overflow:s")).toBe(true);
    // The navigation bar stays where the device shows it, over the list.
    expect(items.get("s/nav#paint0")?.y).toBe(844 - 114);
  });

  test("a drawer shown open covers the page over a scrim, at the start; the end drawer at the end", () => {
    const drawer: WidgetNode = {
      id: "dr",
      type: "Drawer",
      slots: { child: { id: "dc", type: "Column", children: [text("d1", "Menu")] } },
    };
    const open = (side: string): WidgetNode =>
      ({
        id: "sc",
        type: "Scaffold",
        "x-studio": { open: side },
        slots: { body: text("b", "Body"), [side]: { ...drawer } },
      }) as unknown as WidgetNode;
    const start = lay(open("drawer")).items;
    expect(start.get("s/dr#paint0")).toMatchObject({ x: 0, y: 0, w: 390, h: 844, fill: 0x0000008a });
    expect(start.get("s/dr#paint1")).toMatchObject({ x: 0, y: 0, w: 304, h: 844 });
    const end = lay(open("endDrawer")).items;
    expect(end.get("s/dr#paint1")).toMatchObject({ x: 390 - 304, w: 304 });
    // A list in the drawer starts below the status bar, as a ListView pads by the view padding.
    const listed = lay({
      id: "sc",
      type: "Scaffold",
      "x-studio": { open: "drawer" },
      slots: {
        body: text("b", "Body"),
        drawer: { id: "dr", type: "Drawer", slots: { child: { id: "dl", type: "ListView", children: rows(2) } } },
      },
    } as unknown as WidgetNode).items;
    expect((listed.get("s/r0") as Box | undefined)?.y).toBe(47);
    // Not shown: the drawer is not laid out at all.
    const closed = lay({ id: "sc", type: "Scaffold", slots: { body: text("b", "Body"), drawer } }).items;
    expect(closed.has("s/dr#paint0")).toBe(false);
  });

  test("persistent footer buttons sit at the bottom, ending the row, above the home indicator", () => {
    const button = (id: string, label: string): WidgetNode => ({
      id,
      type: "TextButton",
      slots: { child: text(`${id}t`, label) },
    });
    const { items } = lay({
      id: "sc",
      type: "Scaffold",
      slots: {
        body: text("b", "Body"),
        persistentFooterButtons: [button("ok", "OK"), button("no", "Cancel")] as unknown as WidgetNode,
      },
    });
    const ok = items.get("s/ok") as Box | undefined;
    const no = items.get("s/no") as Box | undefined;
    expect(ok !== undefined && no !== undefined).toBe(true);
    expect((no?.x ?? 0) + (no?.w ?? 0)).toBeCloseTo(390 - 8, 0);
    expect((ok?.y ?? 0) + (ok?.h ?? 0)).toBeLessThanOrEqual(844 - 34 - 8 + 0.5);
  });
});
