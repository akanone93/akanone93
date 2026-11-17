// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { declaredNativeRoutes, unionRoutes } from "./use-native-widgets.ts";

const encode = (json: unknown) => new TextEncoder().encode(JSON.stringify(json));

describe("[STU-040] native routes the canvases draw", () => {
  test("the project's catalogue declares routes before any host build is uploaded", () => {
    const docs = [
      { kind: "app", content: encode({ kind: "app" }) },
      {
        kind: "nativeCatalogue",
        content: encode({ kind: "nativeCatalogue", routes: [{ name: "sign-in" }, { name: "licenses" }] }),
      },
      { kind: "nativeCatalogue", content: new TextEncoder().encode("{ not json") },
    ];
    expect(declaredNativeRoutes(docs)).toEqual(["licenses", "sign-in"]);
    expect(declaredNativeRoutes([])).toEqual([]);
  });

  test("a host build's routes and the declared ones are drawn once each", () => {
    expect(unionRoutes(["sign-in", "scanner"], ["licenses", "sign-in"])).toEqual(["licenses", "scanner", "sign-in"]);
  });
});
