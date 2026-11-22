// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { GHOST, placeGhosts, targetKey } from "./ghosts.ts";
import { positionsDocument, positionsOf, positionsRecipe } from "./positions.ts";

const other = "01a0f752-7d75-72a3-9a43-472f6fbc3b6a";

describe("[STU-050] [STU-052] [STU-054] the places a plugin canvas saves (#228)", () => {
  test("screens and other plugins' screens by page id, native routes by route name, read back alike", () => {
    const places = new Map([
      ["s1", { x: 0.4, y: 10 }],
      [other, { x: 900, y: 0 }],
      ["native:sign-in", { x: 900, y: 600 }],
    ]);
    const doc = positionsDocument("c", places);
    expect(doc.positions).toEqual([
      { id: other, x: 900, y: 0 },
      { id: "s1", x: 0, y: 10 },
    ]);
    expect(doc.routes).toEqual([{ route: "sign-in", x: 900, y: 600 }]);
    expect(positionsOf(doc)?.get("native:sign-in")).toEqual({ x: 900, y: 600 });
    expect(positionsOf({ routes: [{ route: "camera", x: 1, y: 2 }] })?.get("native:camera")).toEqual({ x: 1, y: 2 });
  });

  test("a save without native routes drops the list, and keeps the rest of the document", () => {
    const draft: Record<string, unknown> = { id: "c", routes: [{ route: "old", x: 0, y: 0 }], extra: 1 };
    positionsRecipe(new Map([["s1", { x: 1, y: 1 }]]))(draft);
    expect(draft["routes"]).toBeUndefined();
    expect(draft["extra"]).toBe(1);
  });

  test("a target with a saved place stands there; the others stack in the column beside the screens", () => {
    const targets = [
      { id: `ghost:${other}`, kind: "screen" as const, title: "Card", route: "card", screenId: other, links: 1 },
      { id: "native:sign-in", kind: "native" as const, title: "sign-in", route: "sign-in", links: 1 },
    ];
    expect(targets.map(targetKey)).toEqual([other, "native:sign-in"]);
    const placed = placeGhosts(targets, 1000, 0, new Map([["native:sign-in", { x: -500, y: 40 }]]));
    expect(placed[0]).toMatchObject({ x: 1000 + GHOST.offset, y: 0 });
    expect(placed[1]).toMatchObject({ x: -500, y: 40 });
  });
});
