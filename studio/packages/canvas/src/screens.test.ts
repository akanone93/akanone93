// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import { argbToRgba, colorSchemeFromSeed, defaultScreenTheme, TextDirection, type WidgetNode } from "@plux/layout";
import type { CanvasKit } from "canvaskit-wasm";
import { fontBytes, loadCanvasKit, TestEnvironment } from "../test-support/canvaskit.ts";
import { blurSigma, faded } from "./painted.ts";
import { type ScreenStyle, PROTOCOL_VERSION as v, type WorkerMessage } from "./protocol.ts";
import { textLayoutWidth } from "./renderer.ts";
import { layoutScreens, ScreenLayouts } from "./screens.ts";
import { CanvasWorkerCore, canvasParagraphEngine, fallbackFamilies } from "./worker.ts";

let ck: CanvasKit;
beforeAll(async () => {
  ck = await loadCanvasKit();
});

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
};

const text = (id: string, data: string): WidgetNode => ({ id, type: "Text", props: { data } });

describe("ADR-0013 screens laid out in the canvas worker [STU-005] [STU-056]", () => {
  test("a screens message lays documents out with SkParagraph and answers with ops and heights", async () => {
    const env = new TestEnvironment(ck);
    const posted: WorkerMessage[] = [];
    const core = new CanvasWorkerCore((m) => posted.push(m), env.deps());
    await core.receive({
      v,
      type: "init",
      canvas: { width: 0, height: 0 },
      width: 320,
      height: 200,
      dpr: 1,
      options: {
        renderer: "raster",
        tileSize: 256,
        background: 0xffffffff,
        fonts: [fontBytes()],
        fontFamilies: ["Noto Sans"],
        fontFamily: "Noto Sans",
        measureFrames: false,
      },
    });
    const tall: WidgetNode = {
      id: "col",
      type: "Column",
      children: Array.from({ length: 40 }, (_, i) => text(`t${i}`, `Row ${i}`)),
    };
    const odd: WidgetNode = { id: "col", type: "Column", children: [text("a", "Hello"), { id: "m", type: "MapView" }] };
    await core.receive({
      v,
      type: "screens",
      requestId: 9,
      style,
      screens: [
        { id: "long", title: "Long", x: 0, y: 0, root: tall },
        { id: "short", title: "Short", x: 500, y: 0, root: odd },
      ],
    });
    const reply = posted.find((m): m is Extract<WorkerMessage, { type: "screens" }> => m.type === "screens");
    expect(reply?.requestId).toBe(9);
    const long = reply?.heights.find((h) => h.id === "long")?.contentHeight ?? 0;
    expect(long).toBeGreaterThan(844);
    expect(reply?.heights.find((h) => h.id === "short")?.contentHeight).toBeLessThan(844);
    const items = new Map(
      (reply?.ops ?? []).flatMap((op) => (op.op === "add" ? [[op.item.id, op.item] as const] : [])),
    );
    expect(items.get("overflow:long")).toMatchObject({ kind: "rect", y: 844, h: long - 844 });
    expect(items.has("overflow:short")).toBe(false);
    expect(items.get("short/a")).toMatchObject({
      kind: "text",
      text: "Hello",
      x: 516,
      y: 16,
      frame: "screen:short",
      families: ["Noto Sans"],
    });
    expect(items.get("short/m")).toMatchObject({ kind: "rect", label: "MapView", fill: 0x44 });
    // A labelled box draws its label centred on one line inside it.
    expect(items.get("short/m#label")).toMatchObject({ kind: "text", text: "MapView", align: "center", maxLines: 1 });
    const beyond = items.get("long/t39");
    expect(beyond?.kind === "text" && beyond.color).toBe(0x88);
    expect(beyond && "frame" in beyond).toBe(false);
    await core.receive({ v, type: "snapshot", requestId: 1 });
    const snapshot = posted.find((m): m is Extract<WorkerMessage, { type: "snapshot" }> => m.type === "snapshot");
    expect(snapshot?.model.frames).toEqual(["screen:long", "screen:short"]);
    await core.receive({ v, type: "dispose" });
  });

  test("a terms viewer takes 0.6 of the device height of the frame the screen is drawn in", async () => {
    const env = new TestEnvironment(ck);
    const posted: WorkerMessage[] = [];
    const core = new CanvasWorkerCore((m) => posted.push(m), env.deps());
    await core.receive({
      v,
      type: "init",
      canvas: { width: 0, height: 0 },
      width: 320,
      height: 200,
      dpr: 1,
      options: {
        renderer: "raster",
        tileSize: 256,
        background: 0xffffffff,
        fonts: [fontBytes()],
        fontFamilies: ["Noto Sans"],
        fontFamily: "Noto Sans",
        measureFrames: false,
      },
    });
    const terms: WidgetNode = {
      id: "col",
      type: "Column",
      children: [{ id: "terms", type: "TermsViewer", props: { source: "# Terms" } }],
    };
    const heightAt = async (requestId: number, deviceHeight: number): Promise<number> => {
      await core.receive({
        v,
        type: "screens",
        requestId,
        style: { ...style, deviceHeight },
        screens: [{ id: "terms", title: "Terms", x: 0, y: 0, root: terms }],
      });
      const reply = posted.find(
        (m): m is Extract<WorkerMessage, { type: "screens" }> => m.type === "screens" && m.requestId === requestId,
      );
      return reply?.heights.find((h) => h.id === "terms")?.contentHeight ?? 0;
    };
    const phone = await heightAt(1, 844);
    const tall = await heightAt(2, 1000);
    expect(tall - phone).toBeCloseTo(0.6 * (1000 - 844), 5);
    await core.receive({ v, type: "dispose" });
  });

  test("[STU-121] a screensUpdate lays out only the changed screen and touches only its changed items", async () => {
    const env = new TestEnvironment(ck);
    const posted: WorkerMessage[] = [];
    let paragraphs = 0;
    const core = new CanvasWorkerCore((m) => posted.push(m), {
      ...env.deps(),
      paragraphEngine: (c, o) => {
        const engine = canvasParagraphEngine(c, o);
        return {
          buildParagraph: (st, runs) => {
            paragraphs++;
            return engine.buildParagraph(st, runs);
          },
          dispose: () => engine.dispose(),
        };
      },
    });
    await core.receive({
      v,
      type: "init",
      canvas: { width: 0, height: 0 },
      width: 320,
      height: 200,
      dpr: 1,
      options: {
        renderer: "raster",
        tileSize: 256,
        fonts: [fontBytes()],
        fontFamilies: ["Noto Sans"],
        fontFamily: "Noto Sans",
        measureFrames: false,
      },
    });
    const column = (rows: number, edited: string) => ({
      id: "col",
      type: "Column",
      children: Array.from({ length: rows }, (_, i) => text(`t${i}`, i === 3 ? edited : `Row ${i}`)),
    });
    const screens = Array.from({ length: 4 }, (_, n) => ({
      id: `s${n}`,
      title: `S${n}`,
      x: n * 500,
      y: 0,
      root: column(10, "Row 3"),
    }));
    await core.receive({ v, type: "screens", requestId: 1, style, screens });
    const full = paragraphs;
    expect(full).toBeGreaterThanOrEqual(40);
    paragraphs = 0;
    const edited = { ...screens[1], id: "s1", title: "S1", x: 500, y: 0, root: column(10, "Row 3 edited") };
    await core.receive({ v, type: "screensUpdate", requestId: 2, style, screens: [edited], removed: ["s3"] });
    expect(paragraphs).toBe(full / 4);
    const reply = posted.findLast((m): m is Extract<WorkerMessage, { type: "screens" }> => m.type === "screens");
    expect(reply?.requestId).toBe(2);
    expect(reply?.heights.map((h) => h.id)).toEqual(["s1"]);
    const changed = (reply?.ops ?? []).flatMap((op) => (op.op === "replace" ? [op.item.id] : []));
    expect(changed).toContain("s1/t3");
    expect(changed.every((id) => id.startsWith("s1/")) && changed.length <= 2).toBe(true);
    const removed = (reply?.ops ?? []).flatMap((op) => (op.op === "remove" ? [op.id] : []));
    expect(removed).toContain("screen:s3");
    expect(removed.every((id) => id.endsWith("s3") || id.startsWith("s3/"))).toBe(true);
    expect((reply?.ops ?? []).some((op) => op.op === "clear" || op.op === "add")).toBe(false);
    await core.receive({ v, type: "snapshot", requestId: 3 });
    const snapshot = posted.find((m): m is Extract<WorkerMessage, { type: "snapshot" }> => m.type === "snapshot");
    expect(snapshot?.model.frames).toEqual(["screen:s0", "screen:s1", "screen:s2"]);
    await core.receive({ v, type: "dispose" });
  });

  test("the renderer's text falls back across every loaded family after the primary", () => {
    const options = { fontFamily: "Roboto", fontFamilies: ["Roboto", "Roboto", "Noto Sans", "Noto Sans"] };
    expect(fallbackFamilies({ ...style.theme, ...options } as never)).toEqual(["Noto Sans"]);
  });

  test("a text drawn at its laid-out width keeps the lines the layout measured", () => {
    const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
    const measured = engine.buildParagraph(
      { fontSize: 16, fontFamily: "Noto Sans" } as never,
      [{ text: "Personal loan 4.9%", style: null }] as never,
    );
    measured.layout(Number.POSITIVE_INFINITY);
    const width = Math.fround(measured.maxIntrinsicWidth);
    const mgr = ck.FontMgr.FromData(fontBytes());
    const builder = ck.ParagraphBuilder.Make(
      new ck.ParagraphStyle({ textStyle: { color: ck.BLACK, fontSize: 16, fontFamilies: ["Noto Sans"] } }),
      mgr as never,
    );
    builder.addText("Personal loan 4.9%");
    const drawn = builder.build();
    drawn.layout(textLayoutWidth(width));
    expect(drawn.getLineMetrics()).toHaveLength(1);
    drawn.delete();
    builder.delete();
    mgr?.delete();
    engine.dispose();
  });

  test("with a title colour, each screen's name is drawn above its frame on one line", () => {
    const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
    const named = layoutScreens(
      [{ id: "home", title: "Home", x: 0, y: 0, root: text("a", "Hello") }],
      { ...style, title: 0xabcdefff },
      engine,
    );
    const title = named.ops.flatMap((op) => (op.op === "add" ? [op.item] : [])).find((i) => i.id === "title:home");
    expect(title).toMatchObject({ kind: "text", text: "Home", y: -30, weight: "bold", color: 0xabcdefff, maxLines: 1 });
    const plain = layoutScreens([{ id: "home", title: "Home", x: 0, y: 0, root: text("a", "Hello") }], style, engine);
    expect(plain.ops.some((op) => op.op === "add" && op.item.id === "title:home")).toBe(false);
    engine.dispose();
  });

  test("[STU-005] with a paint theme, widgets are drawn as they paint: boxes, texts in onSurface, layout widgets empty", () => {
    const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
    const scheme = colorSchemeFromSeed(0xff6750a4, false);
    const root: WidgetNode = {
      id: "col",
      type: "Column",
      children: [
        {
          id: "box",
          type: "Container",
          props: { height: 40, decoration: { color: { $token: "color.primary" }, borderRadius: { all: 8 } } },
        },
        text("t", "Hello"),
      ],
    };
    const painted = layoutScreens([{ id: "s", title: "S", x: 0, y: 0, root }], { ...style, paint: { scheme } }, engine);
    const items = new Map(painted.ops.flatMap((op) => (op.op === "add" ? [[op.item.id, op.item] as const] : [])));
    expect(items.get("s/col")).toMatchObject({ kind: "rect" });
    expect(items.get("s/col")).not.toHaveProperty("fill");
    const box = items.get("s/box#paint0");
    expect(box).toMatchObject({ kind: "rect", fill: argbToRgba(scheme.primary), radii: [8, 8, 8, 8] });
    expect(items.get("s/t")).toMatchObject({ kind: "text", color: argbToRgba(scheme.onSurface) });
    expect((items.get("s/t")?.z ?? 0) > (box?.z ?? 0)).toBe(true);
    expect(blurSigma(0)).toBe(0);
    expect(blurSigma(10)).toBeCloseTo(6.2735);
    expect(faded(0x112233ff, 0.5)).toBe(0x11223380);
    engine.dispose();
  });

  test("[STU-006] a painted text keeps the style it was measured in: role, weight, line height, lines and alignment", () => {
    const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
    const scheme = colorSchemeFromSeed(0xff6750a4, false);
    const root: WidgetNode = {
      id: "col",
      type: "Column",
      children: [
        {
          id: "h",
          type: "Text",
          props: {
            data: "A headline",
            style: { $token: "typography.headlineSmall", fontWeight: "w700", decoration: "underline" },
            textAlign: "center",
            maxLines: 1,
          },
        },
        text("b", "Body"),
      ],
    };
    const painted = layoutScreens([{ id: "s", title: "S", x: 0, y: 0, root }], { ...style, paint: { scheme } }, engine);
    const items = new Map(painted.ops.flatMap((op) => (op.op === "add" ? [[op.item.id, op.item] as const] : [])));
    expect(items.get("s/h")).toMatchObject({
      kind: "text",
      size: 24,
      weight: 700,
      height: 1.33,
      decoration: "underline",
      align: "center",
      maxLines: 1,
    });
    expect(items.get("s/b")).toMatchObject({ kind: "text", size: 14, height: 1.43, letterSpacing: 0.25 });
    engine.dispose();
  });

  test("[STU-006] a rich text is drawn run by run, each span in its style over its parent's", () => {
    const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
    const scheme = colorSchemeFromSeed(0xff6750a4, false);
    const root: WidgetNode = {
      id: "r",
      type: "RichText",
      props: {
        textSpan: {
          text: "Pay ",
          children: [
            { text: "now", style: { fontWeight: "w700", color: { $token: "color.primary" } } },
            { text: " or later", style: { fontStyle: "italic" } },
          ],
        },
      },
    };
    const painted = layoutScreens([{ id: "s", title: "S", x: 0, y: 0, root }], { ...style, paint: { scheme } }, engine);
    const item = painted.ops.flatMap((op) => (op.op === "add" ? [op.item] : [])).find((i) => i.id === "s/r");
    expect(item).toMatchObject({ kind: "text", text: "Pay now or later" });
    const spans = (item as { spans?: readonly Record<string, unknown>[] }).spans ?? [];
    expect(spans.map((r) => r["text"])).toEqual(["Pay ", "now", " or later"]);
    expect(spans[1]).toMatchObject({ weight: 700, color: argbToRgba(scheme.primary) });
    expect(spans[2]).toMatchObject({ italic: true, weight: 400 });
    engine.dispose();
  });

  test("[STU-005] a painted screen is laid out as Flutter lays it out: edge to edge, flex, padding, containers", () => {
    const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
    const scheme = colorSchemeFromSeed(0xff6750a4, false);
    const root: WidgetNode = {
      id: "sc",
      type: "Scaffold",
      slots: {
        appBar: { id: "ab", type: "AppBar" },
        body: {
          id: "col",
          type: "Column",
          props: { crossAxisAlignment: "stretch" },
          children: [
            {
              id: "p",
              type: "Padding",
              props: { padding: { all: { $token: "space.md" } } },
              slots: { child: { id: "t", type: "Text", props: { data: "Centred", textAlign: "center" } } },
            },
            {
              id: "c",
              type: "Container",
              props: { height: 40, margin: { all: 8 }, color: { $token: "color.primary" } },
            },
            {
              id: "r",
              type: "Row",
              props: { mainAxisAlignment: "spaceBetween" },
              children: [text("a", "Left"), text("b", "Right")],
            },
            { id: "e", type: "Expanded", slots: { child: { id: "x", type: "Container" } } },
          ],
        },
      },
    };
    const paint = { scheme, dimensions: { "space.md": 12 } };
    const painted = layoutScreens([{ id: "s", title: "S", x: 0, y: 0, root }], { ...style, paint }, engine);
    const items = new Map(painted.ops.flatMap((op) => (op.op === "add" ? [[op.item.id, op.item] as const] : [])));
    const box = (id: string) => items.get(`s/${id}`) as { x: number; y: number; w: number; h?: number };
    expect(box("col")).toMatchObject({ x: 0, y: 56, w: 390, h: 844 - 56 });
    expect(box("t")).toMatchObject({ x: 12, y: 68, w: 366, align: "center" });
    expect(box("c")).toMatchObject({ y: 100, h: 56 });
    expect(items.get("s/c#paint0")).toMatchObject({ x: 8, y: 108, w: 374, h: 40 });
    expect(box("a").x).toBe(0);
    expect(box("b").x + box("b").w).toBeCloseTo(390, 5);
    expect(box("x").y + (box("x").h ?? 0)).toBeCloseTo(844, 5);
    // The parts a Container is built of are neither drawn nor selected.
    expect([...items.keys()].some((id) => id.startsWith("s/c#") && !id.includes("#paint"))).toBe(false);
    engine.dispose();
  });

  test("[THM-005] an icon is its glyph in its set's fonts, centred in its box, mirrored in RTL when it asks", () => {
    const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
    const scheme = colorSchemeFromSeed(0xff6750a4, false);
    const icons = {
      material: { home: 0xe9b2, arrow_back: 0xe5c4 },
      cupertino: {},
      families: { material: ["M1", "M2"], cupertino: ["C"] },
      mirrored: ["material arrow_back"],
    };
    const root: WidgetNode = {
      id: "row",
      type: "Row",
      children: [
        { id: "h", type: "Icon", props: { icon: { name: "home" }, size: 32 } },
        { id: "b", type: "Icon", props: { icon: { name: "arrow_back" } } },
        { id: "x", type: "Icon", props: { icon: { name: "no_such_icon" } } },
      ],
    };
    const lay = (dir: TextDirection) =>
      new Map(
        new ScreenLayouts(icons)
          .replaceAll(
            [{ id: "s", title: "S", x: 0, y: 0, root }],
            { ...style, textDirection: dir, paint: { scheme } },
            engine,
          )
          .ops.flatMap((op) => (op.op === "add" ? [[op.item.id, op.item] as const] : [])),
      );
    const ltr = lay(TextDirection.ltr);
    expect(ltr.get("s/h#paintglyph")).toMatchObject({
      kind: "text",
      text: String.fromCodePoint(0xe9b2),
      size: 32,
      height: 1,
      families: ["M1", "M2"],
      color: argbToRgba(0xdd000000),
    });
    expect(ltr.get("s/b#paintglyph")).not.toHaveProperty("mirror");
    expect(ltr.get("s/x#paintglyph")).toBeUndefined();
    expect(lay(TextDirection.rtl).get("s/b#paintglyph")).toMatchObject({ mirror: true });
    engine.dispose();
  });

  test("[AST-002] an image is its asset's key, fitted, over a placeholder; a web image shows its placeholder glyph", () => {
    const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
    const scheme = colorSchemeFromSeed(0xff6750a4, false);
    const icons = {
      material: { image: 0xe3f4 },
      cupertino: {},
      families: { material: ["M"], cupertino: [] },
      mirrored: [],
    };
    const root: WidgetNode = {
      id: "col",
      type: "Column",
      children: [
        {
          id: "a",
          type: "Image",
          props: { source: { asset: { $asset: "logo" } }, fit: "cover", width: 80, height: 40 },
        },
        { id: "w", type: "Image", props: { source: { url: "https://example.com/a.png" }, width: 80, height: 40 } },
      ],
    };
    const items = new Map(
      new ScreenLayouts(icons)
        .replaceAll([{ id: "s", title: "S", x: 0, y: 0, root }], { ...style, paint: { scheme } }, engine)
        .ops.flatMap((op) => (op.op === "add" ? [[op.item.id, op.item] as const] : [])),
    );
    expect(items.get("s/a#paint0")).toMatchObject({
      kind: "rect",
      image: "asset:logo",
      imageFit: { fit: "cover", alignX: 0, alignY: 0, placeholder: style.box },
    });
    expect(items.get("s/w#paint0")).not.toHaveProperty("image");
    expect(items.get("s/w#paint0web")).toMatchObject({ kind: "text", text: String.fromCodePoint(0xe3f4) });
    engine.dispose();
  });

  test("[STU-005] a button is laid out by Flutter's rules and painted as Material 3's: its label in labelLarge, onPrimary", () => {
    const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
    const scheme = colorSchemeFromSeed(0xff6750a4, false);
    const root: WidgetNode = {
      id: "col",
      type: "Column",
      children: [
        {
          id: "b",
          type: "FilledButton",
          events: { onPressed: { steps: [] } },
          slots: { child: { id: "l", type: "Text", props: { data: "Go" } } },
        },
      ],
    };
    const painted = layoutScreens([{ id: "s", title: "S", x: 0, y: 0, root }], { ...style, paint: { scheme } }, engine);
    const items = new Map(painted.ops.flatMap((op) => (op.op === "add" ? [[op.item.id, op.item] as const] : [])));
    expect(items.get("s/b#paint0")).toMatchObject({ kind: "rect", h: 40, fill: argbToRgba(scheme.primary) });
    expect(items.get("s/l")).toMatchObject({
      kind: "text",
      size: 14,
      weight: 500,
      letterSpacing: 0.1,
      color: argbToRgba(scheme.onPrimary),
    });
    engine.dispose();
  });

  test("[STA-020] a text field is laid out by the scene and drawn with its label, value and helper", () => {
    const engine = canvasParagraphEngine(ck, { fonts: [fontBytes()], fontFamily: "Noto Sans" } as never);
    const scheme = colorSchemeFromSeed(0xff6750a4, false);
    const root: WidgetNode = {
      id: "col",
      type: "Column",
      props: { crossAxisAlignment: "stretch" },
      children: [
        { id: "f", type: "TextField", props: { value: "Ada", decoration: { labelText: "Name", helperText: "Yours" } } },
      ],
    };
    const painted = layoutScreens([{ id: "s", title: "S", x: 0, y: 0, root }], { ...style, paint: { scheme } }, engine);
    const texts = painted.ops.flatMap((op) =>
      op.op === "add" && op.item.kind === "text" && op.item.id.startsWith("s/f#") ? [op.item.text] : [],
    );
    expect(texts.sort()).toEqual(["Ada", "Name", "Yours"]);
    engine.dispose();
  });
});
