// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { isKey, isName, keyFromName } from "./keys.ts";

describe("keys as the server checks them [STU-015]", () => {
  test("a key is lower-case letters, digits and inner hyphens, 1 to 64 long", () => {
    expect(["a", "a1", "my-app", "0-9"].every(isKey)).toBe(true);
    expect(["", "-a", "a-", "My", "a_b", "a b", "ä"].some(isKey)).toBe(false);
    expect(isKey("a".repeat(64))).toBe(true);
    expect(isKey("a".repeat(65))).toBe(false);
  });

  test("a name is 1 to 200 bytes after trimming", () => {
    expect(isName("  Shop ")).toBe(true);
    expect(isName("   ")).toBe(false);
    expect(isName("a".repeat(200))).toBe(true);
    expect(isName("a".repeat(201))).toBe(false);
    expect(isName("ä".repeat(101))).toBe(false);
  });

  test("a suggested key always has the key form when the name has a letter or digit", () => {
    expect(keyFromName("  Acme Bank, Inc.  ")).toBe("acme-bank-inc");
    expect(keyFromName("Café Ünï")).toBe("cafe-uni");
    expect(isKey(keyFromName("x".repeat(80)))).toBe(true);
    expect(keyFromName("***")).toBe("");
  });
});
