// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { chooseCurrentApp, rememberApp, rememberedApp } from "./current-app.ts";

afterEach(() => globalThis.localStorage?.clear());

describe("[#202] the app the navigation is about", () => {
  const readable = ["a1", "a2", "a3"];

  test("the address wins, then the page's filter, then the remembered app, then the first app", () => {
    expect(chooseCurrentApp({ inPath: "a2", inSearch: "a3", remembered: "a1", readable })).toBe("a2");
    expect(chooseCurrentApp({ inPath: undefined, inSearch: "a3", remembered: "a1", readable })).toBe("a3");
    expect(chooseCurrentApp({ inPath: undefined, inSearch: undefined, remembered: "a2", readable })).toBe("a2");
    expect(chooseCurrentApp({ inPath: undefined, inSearch: undefined, remembered: undefined, readable })).toBe("a1");
  });

  test("a filter or remembered value names only an app the server listed for the person", () => {
    expect(chooseCurrentApp({ inPath: undefined, inSearch: "other", remembered: "gone", readable })).toBe("a1");
    expect(chooseCurrentApp({ inPath: undefined, inSearch: "x", remembered: "y", readable: [] })).toBe(undefined);
  });

  test("the remembered app is kept per organisation and ignores values it did not write", () => {
    rememberApp("org1", "a2");
    rememberApp("org2", "a3");
    expect([rememberedApp("org1"), rememberedApp("org2"), rememberedApp("org3")]).toEqual(["a2", "a3", undefined]);
    rememberApp("org1", "x".repeat(65));
    expect(rememberedApp("org1")).toBe("a2");
    globalThis.localStorage?.setItem("plux.studio.app.org1", "y".repeat(65));
    expect(rememberedApp("org1")).toBe(undefined);
  });
});
