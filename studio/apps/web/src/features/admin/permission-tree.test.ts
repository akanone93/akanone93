// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { buildTree, toggleAll } from "./permission-tree.ts";

const entry = (name: string) => ({ name, description: "", risk: "", stepUp: false });

describe("[STU-116] role builder tree", () => {
  test("groups area, feature and action in a stable order", () => {
    const tree = buildTree([
      entry("release.publish"),
      entry("function.deploy.production"),
      entry("function.deploy"),
      entry("app.read"),
    ]);
    expect(tree.map((a) => a.area)).toEqual(["app", "function", "release"]);
    const fn = tree[1]?.features.map((f) => [f.feature, f.actions.map((x) => x.action)]);
    expect(fn).toEqual([
      ["", ["deploy"]],
      ["deploy", ["production"]],
    ]);
  });

  test("toggleAll adds and removes without touching the input", () => {
    const start = new Set(["a"]);
    expect([...toggleAll(start, ["b", "c"], true)].sort()).toEqual(["a", "b", "c"]);
    expect([...toggleAll(start, ["a"], false)]).toEqual([]);
    expect([...start]).toEqual(["a"]);
  });
});
