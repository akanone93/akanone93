// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { nodeIdAtPath, resolveNodeParam } from "./node-path.ts";
import type { Node } from "./plugin-model.ts";

const root: Node = {
  id: "n-root",
  type: "column",
  children: [
    { id: "n-a", type: "text" },
    { id: "n-b", type: "row", children: [{ id: "n-c", type: "text" }] },
  ],
};
const ids = new Set(["n-root", "n-a", "n-b", "n-c"]);

describe("[ANL-040] mapping a runtime node path to a document node ID", () => {
  test("root names the root and each ordinal descends by child index", () => {
    expect(nodeIdAtPath(root, "root")).toBe("n-root");
    expect(nodeIdAtPath(root, "root/0")).toBe("n-a");
    expect(nodeIdAtPath(root, "root/1/0")).toBe("n-c");
  });

  test("slots come before children, as in the compiler's source map", () => {
    const withSlot: Node = { id: "r", slots: { header: { id: "h" } }, children: [{ id: "k" }] };
    expect(nodeIdAtPath(withSlot, "root/0")).toBe("h");
    expect(nodeIdAtPath(withSlot, "root/1")).toBe("k");
  });

  test("paths outside the tree or of another form map to nothing", () => {
    expect(nodeIdAtPath(root, "root/5")).toBeUndefined();
    expect(nodeIdAtPath(root, "root/0/0")).toBeUndefined();
    expect(nodeIdAtPath(root, "/pages/home/0")).toBeUndefined();
    expect(nodeIdAtPath(root, "n-a")).toBeUndefined();
    expect(nodeIdAtPath(undefined, "root")).toBeUndefined();
  });

  test("a node ID wins, a path is mapped in the focused screen, anything else selects nothing", () => {
    expect(resolveNodeParam("n-b", ids, { root })).toBe("n-b");
    expect(resolveNodeParam("root/1/0", ids, { root })).toBe("n-c");
    expect(resolveNodeParam("root/1/0", ids, undefined)).toBeUndefined();
    expect(resolveNodeParam("root/9", ids, { root })).toBeUndefined();
  });
});
