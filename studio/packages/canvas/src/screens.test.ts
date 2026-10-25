// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import { defaultScreenTheme, type WidgetNode } from "@plux/layout";
import type { CanvasKit } from "canvaskit-wasm";
import { fontBytes, loadCanvasKit, TestEnvironment } from "../test-support/canvaskit.ts";
import { type ScreenStyle, PROTOCOL_VERSION as v, type WorkerMessage } from "./protocol.ts";
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

describe("ADR-0013 screens laid out in the canvas worker (STU-005, STU-056)", () => {
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
    expect(items.get("short/a")).toMatchObject({ kind: "text", text: "Hello", x: 516, y: 16, frame: "screen:short" });
    expect(items.get("short/m")).toMatchObject({ kind: "rect", label: "MapView", fill: 0x44 });
    const beyond = items.get("long/t39");
    expect(beyond?.kind === "text" && beyond.color).toBe(0x88);
    expect(beyond && "frame" in beyond).toBe(false);
    await core.receive({ v, type: "snapshot", requestId: 1 });
    const snapshot = posted.find((m): m is Extract<WorkerMessage, { type: "snapshot" }> => m.type === "snapshot");
    expect(snapshot?.model.frames).toEqual(["screen:long", "screen:short"]);
    await core.receive({ v, type: "dispose" });
  });

  test("STU-121 a screensUpdate lays out only the changed screen and touches only its changed items", async () => {
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
});
