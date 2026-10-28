// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { isDark } from "./color-scheme.ts";

afterEach(() => document.documentElement.removeAttribute("data-theme"));

describe("STU-011 third-party widgets follow the applied theme", () => {
  test("the theme on the document wins", () => {
    document.documentElement.setAttribute("data-theme", "dark");
    expect(isDark()).toBe(true);
    document.documentElement.setAttribute("data-theme", "light");
    expect(isDark()).toBe(false);
  });

  test("a contrast theme is not dark", () => {
    document.documentElement.setAttribute("data-theme", "contrast");
    expect(isDark()).toBe(false);
  });
});
