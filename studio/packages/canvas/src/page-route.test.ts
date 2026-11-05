// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import { argbToRgba, colorSchemeFromSeed, defaultScreenTheme, ROUTE_TYPE, type WidgetNode } from "@plux/layout";
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
type Box = { x: number; y: number; w: number; h: number };
const text = (id: string, data: string): WidgetNode => ({ id, type: "Text", props: { data } });
const content: WidgetNode = {
  id: "c",
  type: "Padding",
  props: { padding: { all: 24 } },
  slots: {
    child: { id: "col", type: "Column", props: { mainAxisSize: "min" }, children: [text("t", "Delete this card?")] },
  },
};
const opener: WidgetNode = {
  id: "under:sc",
  type: "Scaffold",
  slots: {
    appBar: { id: "under:bar", type: "AppBar", slots: { title: text("under:title", "Cards") } },
    body: text("under:b", "Body"),
  },
};

function lay(root: WidgetNode): Map<string, ItemSpec & Box> {
  const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
  try {
    const out = layoutScreens([{ id: "s", title: "S", x: 0, y: 0, root }], style, engine);
    return new Map(
      out.ops.flatMap((op) => (op.op === "add" ? [[op.item.id, op.item as ItemSpec & Box] as const] : [])),
    );
  } finally {
    engine.dispose();
  }
}

const route = (kind: string, below?: WidgetNode): WidgetNode =>
  ({
    id: "r",
    type: ROUTE_TYPE,
    props: { kind },
    slots: below === undefined ? { page: content } : { below, page: content },
  }) as WidgetNode;

describe("PC-13 dialog and sheet pages presented as the runtime presents them [SCH-022]", () => {
  test("a dialog: the opener under a scrim, the page centred on a 28-corner card at least 280 wide", () => {
    const items = lay(route("dialog", opener));
    // The opener is drawn as its page: its app bar under the status bar.
    expect(items.get("s/under:bar#paint0")).toMatchObject({ y: 0, h: 56 + 47 });
    const scrim = items.get("s/c#paint0");
    expect(scrim).toMatchObject({ x: 0, y: 0, w: 390, h: 844, fill: 0x0000008a });
    const card = items.get("s/c#paint1");
    expect(card).toMatchObject({ radii: [28, 28, 28, 28], fill: argbToRgba(scheme.surfaceContainerHigh) });
    expect(card?.w).toBeGreaterThanOrEqual(280);
    // Centred between the safe area's insets plus 40 by 24.
    expect(card?.x).toBeCloseTo((390 - (card?.w ?? 0)) / 2, 0);
    const middle = 47 + 24 + (844 - 47 - 34 - 48) / 2;
    expect((card?.y ?? 0) + (card?.h ?? 0) / 2).toBeCloseTo(middle, 0);
    // The page's text is drawn above the scrim and the card.
    expect((items.get("s/t")?.z ?? 0) > (card?.z ?? 0)).toBe(true);
  });

  test("a bottom sheet: as wide as the screen up to 640, at the bottom, its top corners rounded", () => {
    const items = lay(route("bottomSheet"));
    const sheet = items.get("s/c#paint1");
    expect(sheet).toMatchObject({ x: 0, w: 390, radii: [28, 28, 0, 0], fill: argbToRgba(scheme.surfaceContainerLow) });
    expect((sheet?.y ?? 0) + (sheet?.h ?? 0)).toBe(844);
    expect(items.get("s/c#paint0")).toMatchObject({ fill: 0x0000008a });
  });
});
