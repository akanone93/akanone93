// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import { argbToRgba, colorSchemeFromSeed, defaultScreenTheme, type WidgetNode } from "@plux/layout";
import type { CanvasKit } from "canvaskit-wasm";
import { fontBytes, loadCanvasKit } from "../test-support/canvaskit.ts";
import { backdropAt, chromeId, chromeItems, glyphColour } from "./chrome.ts";
import type { DeviceChromeSpec, ItemSpec, ScreenStyle } from "./protocol.ts";
import { layoutScreens } from "./screens.ts";
import { canvasParagraphEngine } from "./worker.ts";

let ck: CanvasKit;
beforeAll(async () => {
  ck = await loadCanvasKit();
});

const ON_LIGHT = 0x000000e6;
const ON_DARK = 0xffffffff;

const iphone: DeviceChromeSpec = {
  top: 47,
  right: 0,
  bottom: 34,
  left: 0,
  cutout: "notch",
  indicator: "home",
  platform: "cupertino",
  radius: 47,
  landscape: false,
  time: "9:41",
  visible: true,
  onLight: ON_LIGHT,
  onDark: ON_DARK,
  cutoutColor: 0x000000ff,
};

const pixel: DeviceChromeSpec = {
  ...iphone,
  top: 32,
  bottom: 24,
  cutout: "punch",
  indicator: "gesture",
  platform: "material",
  radius: 28,
};

const doc = { id: "s", title: "Home", x: 100, y: 200, root: null };
const size = { width: 390, height: 844 };
const byPart = (items: readonly ItemSpec[]) => new Map(items.map((i) => [i.id.slice(i.id.indexOf("#") + 1), i]));

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
  title: 0x123456ff,
};

describe("PC-11 device chrome drawn over each screen [STU-057]", () => {
  test("an iPhone with a notch: the time in the left ear, signal, Wi-Fi and battery, the notch, the home indicator", () => {
    const items = chromeItems(doc, iphone, "screen:s", size, [], 0xffffffff, ["Noto Sans"]);
    const parts = byPart(items);
    expect(parts.get("time")).toMatchObject({ kind: "text", text: "9:41", align: "center", color: ON_LIGHT });
    // The left ear is the width beside the 162 wide notch.
    expect(parts.get("time")).toMatchObject({ x: 100, w: (390 - 162) / 2 });
    expect(parts.get("cutout")).toMatchObject({ kind: "rect", x: 100 + 114, y: 200, w: 162, h: 32 });
    expect(parts.get("indicator")).toMatchObject({ w: 134, h: 5, y: 200 + 844 - 13, x: 100 + 128 });
    for (const part of ["signal0", "signal3", "wifi0", "wifi2", "battery", "battery-level"]) {
      expect(parts.has(part)).toBe(true);
    }
    // Every part is clipped to the frame and drawn above the page, and is no document node.
    expect(items.every((i) => i.kind !== "link" && i.kind !== "frame" && i.frame === "screen:s" && i.z >= 9000)).toBe(
      true,
    );
    expect(items[0]?.id.startsWith(chromeId("s", ""))).toBe(true);
  });

  test("Android: the time at the start, an upright battery, a punch-hole camera and the gesture handle", () => {
    const parts = byPart(chromeItems(doc, pixel, "screen:s", size, [], 0xffffffff, ["Noto Sans"]));
    expect(parts.get("time")).toMatchObject({ kind: "text", x: 116, size: 14, weight: 500 });
    expect(parts.get("cutout")).toMatchObject({ oval: true, w: 12, h: 12, x: 100 + 189, y: 200 + 10 });
    expect(parts.get("indicator")).toMatchObject({ w: 108, h: 4, y: 200 + 844 - 12 - 2 });
    expect(parts.get("battery")).toMatchObject({ w: 8, h: 14 });
  });

  test("a foldable's crease: a shaded band and a hairline down the middle, across it in landscape (PC-33)", () => {
    const fold = { ...pixel, fold: "vertical" as const };
    const parts = byPart(
      chromeItems(doc, fold, "screen:s", { width: 600, height: 800 }, [], 0xffffffff, ["Noto Sans"]),
    );
    expect(parts.get("fold-line")).toMatchObject({ kind: "rect", x: 399.5, y: 200, w: 1, h: 800 });
    expect(parts.get("fold")).toMatchObject({ x: 388, w: 24, h: 800 });
    const flat = byPart(
      chromeItems(doc, { ...pixel, fold: "horizontal" }, "screen:s", { width: 800, height: 600 }, [], 0xffffffff, []),
    );
    expect(flat.get("fold-line")).toMatchObject({ x: 100, y: 499.5, w: 800, h: 1 });
    expect(byPart(chromeItems(doc, pixel, "screen:s", size, [], 0xffffffff, [])).has("fold")).toBe(false);
  });

  test("the icons read over the page: light over a dark app bar, dark over a light page", () => {
    const dark = 0x1d1b20ff;
    const bar = {
      id: "bar",
      kind: "box" as const,
      x: 0,
      y: 0,
      w: 390,
      h: 103,
      paint: {
        role: "paint" as const,
        boxes: [{ x: 0, y: 0, w: 390, h: 103, fill: dark }],
        order: 3,
        opacity: 1,
        clips: [],
      },
    };
    expect(backdropAt([bar], 195, 20, 0xffffffff)).toBe(dark);
    expect(backdropAt([bar], 195, 800, 0xffffffff)).toBe(0xffffffff);
    expect(glyphColour(iphone, dark)).toBe(ON_DARK);
    expect(glyphColour(iphone, 0xfef7ffff)).toBe(ON_LIGHT);
    const parts = byPart(chromeItems(doc, iphone, "screen:s", size, [bar], 0xffffffff, ["Noto Sans"]));
    expect(parts.get("time")).toMatchObject({ color: ON_DARK });
    expect(parts.get("indicator")).toMatchObject({ fill: ON_LIGHT });
  });

  test("hidden chrome draws nothing; iOS in landscape hides the status bar and turns the notch to the side", () => {
    expect(chromeItems(doc, { ...iphone, visible: false }, "screen:s", size, [], 0xffffffff, [])).toEqual([]);
    const wide = { width: 844, height: 390 };
    const parts = byPart(
      chromeItems(
        doc,
        { ...iphone, top: 0, left: 47, right: 47, bottom: 21, landscape: true },
        "screen:s",
        wide,
        [],
        0xffffffff,
        [],
      ),
    );
    expect(parts.has("time")).toBe(false);
    expect(parts.get("cutout")).toMatchObject({ x: 100, w: 30, h: 162, y: 200 + (390 - 162) / 2 });
    expect(parts.get("indicator")).toMatchObject({ w: 200 });
  });

  test("a right-to-left system mirrors the status bar: the time at the end", () => {
    const ltr = byPart(chromeItems(doc, pixel, "screen:s", size, [], 0xffffffff, []));
    const rtl = byPart(chromeItems(doc, pixel, "screen:s", size, [], 0xffffffff, [], true));
    const t = ltr.get("time") as { x: number; w: number };
    expect(rtl.get("time")).toMatchObject({ x: 100 + 390 - (t.x - 100) - t.w });
    expect((rtl.get("battery") as { x: number }).x).toBeLessThan(100 + 195);
    // The Wi-Fi arcs open upwards either way.
    expect(rtl.get("wifi1")).toMatchObject({ arc: { start: 225, sweep: 90 } });
  });

  test("an entry-level phone shows its three navigation buttons", () => {
    const parts = byPart(
      chromeItems(
        doc,
        { ...pixel, cutout: "none", indicator: "buttons", bottom: 48 },
        "screen:s",
        size,
        [],
        0xffffffff,
        [],
      ),
    );
    expect([...parts.keys()].filter((k) => k.startsWith("nav-"))).toEqual(["nav-back", "nav-home", "nav-recents"]);
    expect(parts.has("cutout")).toBe(false);
  });

  test("a screen in a device: its corners, its header's route and kind, its app bar under the status bar", () => {
    const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
    const scheme = colorSchemeFromSeed(0xff6750a4, false);
    const root: WidgetNode = {
      id: "sc",
      type: "Scaffold",
      slots: {
        appBar: { id: "bar", type: "AppBar", slots: { title: { id: "t", type: "Text", props: { data: "Loans" } } } },
        body: {
          id: "col",
          type: "Column",
          children: [
            { id: "safe", type: "SafeArea", children: [{ id: "x", type: "SizedBox", props: { height: 10 } }] },
          ],
        },
      },
    };
    const painted = layoutScreens(
      [{ id: "s", title: "Loans", meta: "loans · Page", x: 0, y: 0, root }],
      { ...style, paint: { scheme }, chrome: iphone },
      engine,
    );
    const items = new Map(painted.ops.flatMap((op) => (op.op === "add" ? [[op.item.id, op.item] as const] : [])));
    expect(items.get("screen:s")).toMatchObject({ kind: "frame", radius: 47 });
    expect(items.get("title:s#meta")).toMatchObject({ kind: "text", text: "loans · Page", align: "end" });
    // The bar's surface reaches under the status bar; its title sits in the toolbar below it.
    expect(items.get("s/bar#paint0")).toMatchObject({ y: 0, h: 56 + 47, fill: argbToRgba(scheme.surface) });
    expect((items.get("s/t") as { y: number }).y).toBeGreaterThan(47);
    // A fill reaching into the frame's corners is cut by them, as the display cuts it; the title is not.
    expect(items.get("s/bar#paint0")).toMatchObject({
      clips: [{ x: 0, y: 0, w: 390, h: 844, radii: [47, 47, 47, 47] }],
    });
    expect("clips" in (items.get("s/t") ?? {})).toBe(false);
    // Below an app bar the body has no status bar to avoid; the home indicator's inset remains.
    const safe = items.get("s/safe") as { y: number; h: number } | undefined;
    expect(safe?.y).toBe(56 + 47);
    expect(safe?.h).toBe(10 + 34);
    expect(items.has("chrome:s#time") && items.has("chrome:s#cutout")).toBe(true);
    engine.dispose();
  });

  test("without an app bar, a SafeArea keeps its content below the status bar", () => {
    const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
    const scheme = colorSchemeFromSeed(0xff6750a4, false);
    const root: WidgetNode = {
      id: "safe",
      type: "SafeArea",
      props: { bottom: false },
      children: [{ id: "x", type: "SizedBox", props: { height: 10 } }],
    };
    const painted = layoutScreens(
      [{ id: "s", title: "S", x: 0, y: 0, root }],
      { ...style, paint: { scheme }, chrome: iphone },
      engine,
    );
    const items = new Map(painted.ops.flatMap((op) => (op.op === "add" ? [[op.item.id, op.item] as const] : [])));
    expect(items.get("s/x")).toMatchObject({ y: 47 });
    expect((items.get("s/safe") as { h: number }).h).toBe(57);
    engine.dispose();
  });
});
