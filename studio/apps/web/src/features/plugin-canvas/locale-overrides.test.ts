// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { overridesFor, ownOverrides, setOverrideRecipe, withLocaleOverrides } from "./locale-overrides.ts";

describe("PC-32 per-locale overrides [I18N-012]", () => {
  test("the runtime's layers: the language's overrides, the exact locale's over them", () => {
    const node = { id: "n", type: "Image", localeOverrides: { de: { a: 1, b: 1 }, "de-AT": { b: 2 } } };
    expect(overridesFor(node, "de-AT")).toEqual({ a: 1, b: 2 });
    expect(overridesFor(node, "de")).toEqual({ a: 1, b: 1 });
    expect(overridesFor(node, "en")).toBeUndefined();
    expect(ownOverrides(node, "de-AT")).toEqual({ b: 2 });
    expect(withLocaleOverrides({ id: "n", props: { a: 0, c: 3 }, localeOverrides: { de: { a: 1 } } }, "de")).toEqual({
      id: "n",
      props: { a: 1, c: 3 },
      localeOverrides: { de: { a: 1 } },
    });
  });

  test("the recipe sets and removes one prop's override and drops what becomes empty", () => {
    const root: Record<string, unknown> = { id: "r", type: "Column", children: [{ id: "n", type: "Text" }] };
    const node = () => (root["children"] as Record<string, unknown>[])[0];
    setOverrideRecipe("n", "de", "width", 120)(root);
    setOverrideRecipe("n", "ar", "source", { asset: "logo-ar" })(root);
    expect(node()?.["localeOverrides"]).toEqual({ de: { width: 120 }, ar: { source: { asset: "logo-ar" } } });
    setOverrideRecipe("n", "de", "width", undefined)(root);
    setOverrideRecipe("n", "ar", "source", undefined)(root);
    expect(node()?.["localeOverrides"]).toBeUndefined();
  });
});
