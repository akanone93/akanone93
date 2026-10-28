// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import type { CanvasKit } from "canvaskit-wasm";
import { fontBytes, loadCanvasKit, TestEnvironment, text } from "../test-support/canvaskit.ts";
import {
  AUTHORITATIVE_NOTICE,
  complexScriptOf,
  type FontLoadDeps,
  type FontRequest,
  type LocalFontLike,
  loadCanvasFonts,
} from "./fonts.ts";
import { CanvasHost, type HostEvent, type WorkerLike } from "./host.ts";
import type { CanvasOptions, HostMessage } from "./protocol.ts";

let ck: CanvasKit;
beforeAll(async () => {
  ck = await loadCanvasKit();
});

const request: FontRequest = {
  device: [{ family: "Brand", url: "/assets/brand.ttf" }],
  roboto: { family: "Roboto", url: "/fonts/roboto.ttf" },
  noto: [{ family: "Noto Sans", url: "/fonts/noto-sans.ttf" }],
  sfPro: false,
};

function fetching(available: readonly string[]): FontLoadDeps & { fetched: string[] } {
  const fetched: string[] = [];
  return {
    fetched,
    fetchFont: (url) => {
      fetched.push(url);
      return available.includes(url) ? Promise.resolve(new ArrayBuffer(url.length)) : Promise.reject(new Error("404"));
    },
  };
}

function sfPro(): LocalFontLike {
  return {
    family: "SF Pro",
    postscriptName: "SFPro-Regular",
    blob: () => Promise.resolve({ arrayBuffer: () => Promise.resolve(new ArrayBuffer(7)) }),
  };
}

describe("canvas fonts [STU-006]", () => {
  test("[STU-006]: host-bundled fonts rank after the app's own and before Roboto; a missing file raises the notice", async () => {
    const host = [
      { family: "HostSerif", url: "/host/serif.ttf" },
      { family: "HostMono", url: "/host/mono.ttf" },
    ];
    const deps = fetching(["/assets/brand.ttf", "/host/serif.ttf", "/fonts/roboto.ttf"]);
    const loaded = await loadCanvasFonts({ ...request, host, noto: [] }, deps);
    expect(loaded.families).toEqual(["Brand", "HostSerif", "Roboto"]);
    expect(loaded.notices).toEqual([{ code: AUTHORITATIVE_NOTICE, reason: "font-unavailable", subject: "HostMono" }]);
  });

  test("[STU-006]: loads device fonts first, then Roboto and Noto, with no notice", async () => {
    const deps = fetching(["/assets/brand.ttf", "/fonts/roboto.ttf", "/fonts/noto-sans.ttf"]);
    const loaded = await loadCanvasFonts(request, deps);
    expect(loaded.families).toEqual(["Brand", "Roboto", "Noto Sans"]);
    expect(loaded.buffers.map((b) => b.byteLength)).toEqual([17, 17, 20]);
    expect(loaded.primary).toBe("Brand");
    expect(loaded.notices).toEqual([]);
  });

  test("[STU-006]: a font that fails to load falls back to the next family, with the notice", async () => {
    const loaded = await loadCanvasFonts(request, fetching(["/fonts/noto-sans.ttf"]));
    expect(loaded.primary).toBe("Noto Sans");
    expect(loaded.notices.map((n) => [n.code, n.reason, n.subject])).toEqual([
      [AUTHORITATIVE_NOTICE, "font-unavailable", "Brand"],
      [AUTHORITATIVE_NOTICE, "font-unavailable", "Roboto"],
    ]);
    const none = await loadCanvasFonts({ ...request, device: [], noto: [] }, fetching([]), "Fallback");
    expect(none.buffers).toEqual([]);
    expect(none.primary).toBe("Fallback");
  });

  test("[STU-006]: SF Pro only through Local Font Access when permitted, never fetched", async () => {
    const deps = fetching(["/fonts/roboto.ttf"]);
    const permitted = await loadCanvasFonts(
      { ...request, device: [], noto: [], sfPro: true },
      { ...deps, queryLocalFonts: () => Promise.resolve([sfPro(), { ...sfPro(), family: "Helvetica" }]) },
    );
    expect(permitted.families).toEqual(["SF Pro", "Roboto"]);
    expect(permitted.notices).toEqual([]);
    expect(deps.fetched.some((url) => /sf/i.test(url))).toBe(false);

    const refused = await loadCanvasFonts(
      { ...request, device: [], noto: [], sfPro: true },
      { ...deps, queryLocalFonts: () => Promise.reject(new DOMException("denied", "NotAllowedError")) },
    );
    expect(refused.families).toEqual(["Roboto"]);
    expect(refused.notices).toEqual([{ code: AUTHORITATIVE_NOTICE, reason: "sf-pro-substitute", subject: "SF Pro" }]);

    const unsupported = await loadCanvasFonts({ ...request, device: [], noto: [], sfPro: true }, deps);
    expect(unsupported.notices.map((n) => n.reason)).toEqual(["sf-pro-substitute"]);
  });

  test("[STU-006]: detects complex scripts whose shaping the device preview decides", () => {
    expect(complexScriptOf("Hello")).toBeNull();
    expect(complexScriptOf("Ünïcödé — ok")).toBeNull();
    expect(complexScriptOf("مرحبا")).toBe("Arabic");
    expect(complexScriptOf("नमस्ते")).toBe("Devanagari");
    expect(complexScriptOf("สวัสดี")).toBe("Thai");
  });

  test("[STU-006]: every locale of the matrix is classified: Latin is reproduced, each complex script raises the notice", () => {
    const samples: readonly (readonly [string, string | null])[] = [
      ["Account settings", null],
      ["إعدادات الحساب", "Arabic"],
      ["הגדרות חשבון", "Hebrew"],
      ["खाता सेटिंग्स", "Devanagari"],
      ["অ্যাকাউন্ট সেটিংস", "Bengali"],
      ["கணக்கு அமைப்புகள்", "Tamil"],
      ["การตั้งค่าบัญชี", "Thai"],
      ["ការកំណត់គណនី", "Khmer"],
      ["အကောင့်ဆက်တင်များ", "Myanmar"],
    ];
    for (const [text, script] of samples) {
      expect(complexScriptOf(text)).toBe(script);
    }
  });

  test("[STU-006]: the host loads fonts before init and raises the notice as a host event", async () => {
    const env = new TestEnvironment(ck);
    const options: CanvasOptions = {
      renderer: "raster",
      tileSize: 256,
      background: 0xffffffff,
      fonts: [],
      fontFamily: "Roboto",
      measureFrames: false,
    };
    const bytes = fontBytes();
    const fonts = {
      request: { device: [], roboto: null, noto: [{ family: "Noto Sans", url: "/noto" }], sfPro: true },
      deps: {
        fetchFont: (url: string) => (url === "/noto" ? Promise.resolve(bytes) : Promise.reject(new Error("404"))),
      },
    };
    // In a worker, the font bytes travel in `init` (transferred), so they are there before any layout.
    const sent: { message: HostMessage; transfer: unknown[] }[] = [];
    const worker: WorkerLike = {
      onmessage: null,
      postMessage: (message, transfer) => sent.push({ message: message as HostMessage, transfer }),
      terminate: () => undefined,
    };
    const element = { width: 0, height: 0, transferControlToOffscreen: () => ({ width: 0, height: 0 }) };
    await CanvasHost.create(
      element,
      { width: 200, height: 100, dpr: 1 },
      options,
      { createWorker: () => worker, mainThread: () => env.deps() },
      fonts,
    );
    const first = sent[0];
    expect(first?.message.type).toBe("init");
    if (first?.message.type === "init") {
      expect(first.message.options.fonts).toEqual([bytes]);
      expect(first.message.options.fontFamily).toBe("Noto Sans");
      expect(first.transfer).toContain(bytes);
    }
    const host = await CanvasHost.create(
      { width: 0, height: 0 },
      { width: 200, height: 100, dpr: 1 },
      options,
      { mainThread: () => env.deps() },
      fonts,
    );
    await host.ready;
    const events: HostEvent[] = [];
    host.onEvent((event) => events.push(event));
    expect(events).toEqual([
      { type: "notice", notice: { code: AUTHORITATIVE_NOTICE, reason: "sf-pro-substitute", subject: "SF Pro" } },
    ]);
    host.patch([{ op: "add", item: text("t", 0, 0, "مرحبا") }]);
    host.patch([{ op: "add", item: text("u", 0, 40, "مرحبا بكم") }]);
    expect(events.map((e) => (e.type === "notice" ? e.notice.reason : e.type))).toEqual([
      "sf-pro-substitute",
      "complex-script",
    ]);
    env.drain();
    const model = await host.snapshot();
    expect(model.nodes.map((n) => n.label)).toEqual(["مرحبا", "مرحبا بكم"]);
    expect(host.raisedNotices).toHaveLength(2);
    host.dispose();
  });
});
