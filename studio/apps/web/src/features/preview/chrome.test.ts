// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { chromeSpec, lookedStyle } from "../plugin-canvas/look.ts";
import { screenDocuments, screenStyle } from "../plugin-canvas/scene.ts";
import { readFrameView } from "./frame-view.ts";
import { customFrame, frameCatalogue, frameChrome, framePadding, resolveFrame } from "./frames.ts";
import { defaultPreview, genericDevice, previewDevice, previewRender } from "./switches.ts";

/** A storage holding one value under the frame view's key. */
function storeWith(value: string | null): Storage {
  return { getItem: () => value } as unknown as Storage;
}

describe("PC-11 device frames with their system areas [STU-057]", () => {
  test("every preset has chrome: iPhones a notch or an island and a home indicator, Androids a punch hole", () => {
    for (const f of frameCatalogue) {
      const c = frameChrome(f);
      expect(c.radius).toBeGreaterThanOrEqual(0);
      expect(c.top).toBeGreaterThanOrEqual(0);
    }
    expect(frameChrome(resolveFrame("iphone"))).toMatchObject({
      top: 47,
      bottom: 34,
      cutout: "notch",
      indicator: "home",
    });
    expect(frameChrome(resolveFrame("iphone-pro-max"))).toMatchObject({ top: 59, cutout: "island" });
    expect(frameChrome(resolveFrame("iphone-se"))).toMatchObject({ top: 20, bottom: 0, cutout: "none", radius: 0 });
    expect(frameChrome(resolveFrame("pixel"))).toMatchObject({ cutout: "punch", indicator: "gesture" });
    expect(frameChrome(resolveFrame("android-entry"))).toMatchObject({ indicator: "buttons", bottom: 48 });
    expect(frameChrome(resolveFrame("desktop"))).toMatchObject({ top: 0, bottom: 0, indicator: "none" });
  });

  test("a custom frame gets a plain device of its kind", () => {
    const phone = customFrame("c", { width: 400, height: 860, density: 2 });
    const desk = customFrame("d", { width: 1440, height: 900, density: 1 });
    expect(phone !== "invalid" && frameChrome(phone)).toMatchObject({ top: 24, cutout: "none" });
    expect(desk !== "invalid" && frameChrome(desk)).toMatchObject({ top: 0, indicator: "none" });
  });

  test("landscape: iOS hides the status bar and pads both sides of the notch; Android keeps its status bar", () => {
    expect(framePadding(resolveFrame("iphone"), "landscape")).toEqual({ top: 0, right: 47, bottom: 21, left: 47 });
    expect(framePadding(resolveFrame("pixel"), "landscape")).toEqual({ top: 32, right: 0, bottom: 24, left: 32 });
    expect(framePadding(resolveFrame("ipad"), "landscape")).toEqual({ top: 24, right: 0, bottom: 20, left: 0 });
    expect(previewDevice(resolveFrame("iphone"), "landscape", true)).toMatchObject({
      width: 844,
      height: 390,
      landscape: true,
    });
  });

  test("the canvas draws the chosen device: its size, corners and chrome reach the screen style", () => {
    const device = previewDevice(resolveFrame("iphone-pro-max"), "portrait", true);
    const look = previewRender(defaultPreview, "cupertino", device);
    const style = lookedStyle(screenStyle, screenStyle.theme, look);
    expect(style.theme.width).toBe(430);
    expect(style.deviceHeight).toBe(932);
    expect(style.chrome).toMatchObject({ top: 59, bottom: 34, cutout: "island", radius: 55, platform: "cupertino" });
    expect(chromeSpec({ ...device, chrome: false }, "material")).toMatchObject({ visible: false, top: 59 });
    // Without a chosen frame the canvas keeps its generic phone, with no chrome.
    const plain = lookedStyle(screenStyle, screenStyle.theme, previewRender(defaultPreview, "material"));
    expect(plain.chrome).toBeUndefined();
    expect(plain.theme.width).toBe(genericDevice.width);
  });

  test("the frame view is remembered in the browser and falls back to portrait with chrome", () => {
    expect(readFrameView(storeWith(JSON.stringify({ orientation: "landscape", chrome: false })))).toEqual({
      orientation: "landscape",
      chrome: false,
    });
    expect(readFrameView(storeWith(null))).toEqual({ orientation: "portrait", chrome: true });
    expect(readFrameView(storeWith("{not json"))).toEqual({ orientation: "portrait", chrome: true });
    expect(readFrameView(undefined)).toEqual({ orientation: "portrait", chrome: true });
  });

  test("a screen's header line names its route and kind", () => {
    const docs = screenDocuments(
      [
        { id: "a", title: "Home", route: "home", kind: "screen", at: { x: 0, y: 0 }, root: undefined },
        { id: "b", title: "Other", at: { x: 500, y: 0 }, root: undefined },
      ],
      (s) => (s.route === undefined ? undefined : `${s.route} · ${s.kind}`),
    );
    expect(docs[0]).toMatchObject({ id: "a", meta: "home · screen" });
    expect("meta" in (docs[1] ?? {})).toBe(false);
  });
});
