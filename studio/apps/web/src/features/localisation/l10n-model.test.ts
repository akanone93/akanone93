// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  addKeyRecipe,
  keyProblem,
  localePath,
  localeScope,
  proposeKey,
  readKeys,
  readLocales,
  readMessages,
  referencedKeys,
  setMessageRecipe,
} from "./l10n-model.ts";

describe("I18N-007 key names", () => {
  test("a proposed key follows the key pattern and is unique", () => {
    const taken = new Set<string>();
    const first = proposeKey("Pay now, safely!", "Button", taken);
    expect(first).toBe("button.pay.now.safely");
    expect(keyProblem(first, taken)).toBeUndefined();
    taken.add(first);
    expect(proposeKey("Pay now, safely!", "Button", taken)).toBe("button.pay.now.safely.2");
  });

  test("text without letters and a numeric prefix still give a valid key", () => {
    expect(keyProblem(proposeKey("???", "3d", new Set()), new Set())).toBeUndefined();
    expect(proposeKey("???", "", new Set())).toBe("text.text");
  });

  test("keyProblem names why a key is refused", () => {
    expect(keyProblem("", new Set())).toBe("empty");
    expect(keyProblem("Home.Title", new Set())).toBe("invalid");
    expect(keyProblem("home.title", new Set(["home.title"]))).toBe("taken");
    expect(keyProblem("a".repeat(129), new Set())).toBe("invalid");
  });
});

describe("I18N-008 documents", () => {
  test("paths and lock scopes of a locale", () => {
    expect(localePath("de")).toBe("translations/de.json");
    expect(localeScope("de")).toBe("translations/de");
  });

  test("keys and messages are read, malformed entries skipped", () => {
    expect(readKeys({ keys: [{ id: "k1", key: "a.b", description: "d" }, { id: 3 }, "x"] })).toEqual([
      { id: "k1", key: "a.b", description: "d" },
    ]);
    expect(readKeys(undefined)).toEqual([]);
    expect(readMessages({ messages: { k1: "Hi", k2: 5 } })).toEqual({ k1: "Hi" });
    expect(readMessages(null)).toEqual({});
  });

  test("the default locale comes first", () => {
    expect(readLocales({ defaultLocale: "en", supportedLocales: ["de", "en", "ar"] })).toEqual({
      defaultLocale: "en",
      supported: ["en", "de", "ar"],
    });
    expect(readLocales({})).toBeUndefined();
  });

  test("references carry the node they sit in", () => {
    const page = {
      root: { id: "n1", type: "Column", children: [{ id: "n2", type: "Text", props: { text: { $t: "k1" } } }] },
    };
    expect(referencedKeys(page)).toEqual([{ id: "k1", nodeId: "n2" }]);
  });

  test("recipes add a key and set or clear a message", () => {
    const keys: Record<string, unknown> = { keys: [] };
    addKeyRecipe({ id: "k1", key: "a.b", description: "why" })(keys);
    expect(keys["keys"]).toEqual([{ id: "k1", key: "a.b", description: "why" }]);
    const messages: Record<string, unknown> = {};
    setMessageRecipe("k1", "Hi")(messages);
    expect(messages["messages"]).toEqual({ k1: "Hi" });
    setMessageRecipe("k1", "")(messages);
    expect(messages["messages"]).toEqual({});
  });
});
