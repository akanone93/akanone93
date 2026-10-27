// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { PreviewSwitchesSchema } from "@plux/api";
import {
  customFrame,
  customFrameLimits,
  customFrameRefusal,
  decodeCustom,
  encodeCustom,
  frameCatalogue,
  matrixLayout,
  orientedSize,
  resolveFrame,
  sizeClassOf,
} from "./frames.ts";
import { defaultPreview, mockStateOf, previewRender, readPreview, withMock, writePreview } from "./switches.ts";

describe("STU-057 device frames", () => {
  test("the catalogue has the named presets with unique IDs", () => {
    const ids = frameCatalogue.map((f) => f.id);
    expect(new Set(ids).size).toBe(ids.length);
    expect(resolveFrame("android-small")).toMatchObject({ width: 360, height: 800 });
    expect(ids).toEqual(expect.arrayContaining(["pixel", "iphone-se", "iphone", "iphone-pro-max", "ipad", "foldable"]));
    expect(frameCatalogue.some((f) => f.platform === "cupertino")).toBe(true);
    expect(frameCatalogue.some((f) => f.platform === "material")).toBe(true);
    expect(frameCatalogue.some((f) => f.kind === "desktop")).toBe(true);
  });
  test("landscape swaps the size and the size class follows the width", () => {
    expect(orientedSize({ width: 360, height: 800 }, "landscape")).toEqual({ width: 800, height: 360 });
    expect([sizeClassOf(360), sizeClassOf(700), sizeClassOf(1280)]).toEqual(["compact", "medium", "expanded"]);
  });
  test("a custom frame needs whole positive pixels and a positive density, and round-trips its stored form", () => {
    expect(customFrame("x", { width: 0, height: 800, density: 2 })).toBe("invalid");
    expect(customFrame("x", { width: 360.5, height: 800, density: 2 })).toBe("invalid");
    expect(customFrame("x", { width: 360, height: 800, density: 0 })).toBe("invalid");
    const f = decodeCustom(encodeCustom({ width: 500, height: 900, density: 2.5 }));
    expect(f).toMatchObject({ width: 500, height: 900, density: 2.5, kind: "phone" });
    expect(decodeCustom("custom:bad")).toBeUndefined();
    expect(resolveFrame("nope").id).toBe("android-small");
  });
  test("a custom frame above studio.customFrameMaxPixels is refused, at the limit is accepted [STU-057]", () => {
    const { maxPixels } = customFrameLimits();
    expect(maxPixels).toBe(4096);
    expect(customFrameRefusal({ width: maxPixels + 1, height: 800, density: 2 })).toBe("size");
    expect(customFrameRefusal({ width: 360, height: maxPixels + 1, density: 2 })).toBe("size");
    expect(customFrame("x", { width: 360, height: maxPixels + 1, density: 2 })).toBe("invalid");
    expect(customFrameRefusal({ width: 360, height: maxPixels, density: 2 })).toBeUndefined();
    expect(decodeCustom("custom:360x9000@2")).toBeUndefined();
  });
  test("a custom frame density outside 1 to studio.customFrameMaxDensity is refused [STU-057]", () => {
    const { maxDensity } = customFrameLimits();
    expect(maxDensity).toBe(4);
    expect(customFrameRefusal({ width: 360, height: 800, density: maxDensity + 0.5 })).toBe("density");
    expect(customFrameRefusal({ width: 360, height: 800, density: 0.5 })).toBe("density");
    expect(customFrame("x", { width: 360, height: 800, density: maxDensity + 0.5 })).toBe("invalid");
    expect(customFrameRefusal({ width: 360, height: 800, density: maxDensity })).toBeUndefined();
    expect(customFrameRefusal({ width: 360, height: 800, density: 1 })).toBeUndefined();
  });
  test("the matrix lays frames side by side in order", () => {
    const frames = [resolveFrame("android-small"), resolveFrame("ipad")];
    const cells = matrixLayout(frames, "portrait", 10);
    expect(cells.map((c) => c.x)).toEqual([0, 370]);
    expect(cells.map((c) => c.sizeClass)).toEqual(["compact", "medium"]);
  });
});

describe("STU-058 preview switches", () => {
  test("unset or unknown stored values read as the defaults", () => {
    expect(readPreview(undefined)).toEqual(defaultPreview);
    const odd = create(PreviewSwitchesSchema, { textScale: 99, platform: "x", theme: "x" });
    expect(readPreview(odd)).toMatchObject({ textScale: 1, platform: "", theme: "light" });
  });
  test("the choice survives a write and a read", () => {
    const state = withMock(
      { ...defaultPreview, theme: "dark", locale: "ar", textScale: 1.5, platform: "cupertino", deviceFrame: "ipad" },
      "orders",
      "error",
    );
    const back = readPreview(writePreview(state));
    expect(back).toEqual(state);
    expect(mockStateOf(back, "orders")).toBe("error");
    expect(mockStateOf(back, "other")).toBe("success");
  });
  test("RTL locales flip the direction, ltr locales do not [I18N-010]", () => {
    expect(previewRender({ ...defaultPreview, locale: "ar" }, "material").direction).toBe("rtl");
    expect(previewRender({ ...defaultPreview, locale: "ar-XB" }, "material").direction).toBe("rtl");
    expect(previewRender({ ...defaultPreview, locale: "en-XA" }, "material").direction).toBe("ltr");
    expect(previewRender({ ...defaultPreview, locale: "de" }, "material").direction).toBe("ltr");
  });
  test("pseudo-locales expand text and other locales leave it alone [I18N-010]", () => {
    const expanded = previewRender({ ...defaultPreview, locale: "en-XA" }, "material").localize("Save changes");
    expect(expanded.length).toBeGreaterThan("Save changes".length);
    expect(previewRender(defaultPreview, "material").localize("Save")).toBe("Save");
  });
  test("text scale and platform reach the render; empty platform follows the frame [A11Y-005]", () => {
    const r = previewRender({ ...defaultPreview, textScale: 1.3 }, "cupertino");
    expect(r).toMatchObject({ textScale: 1.3, platform: "cupertino" });
    expect(previewRender({ ...defaultPreview, platform: "material" }, "cupertino").platform).toBe("material");
  });
});
