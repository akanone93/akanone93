// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { enablePatches, produceWithPatches } from "immer";
import { applyPatch, diffPaths, type JsonValue, PatchError, pointer, toJsonPatch } from "./json-patch.ts";

/** The test page's shape, for typed access inside edit recipes. */
interface Page {
  kind: string;
  props: { padding: number; title?: string; color?: string };
  children: string[];
}

function page(d: Record<string, unknown>): Page {
  return d as unknown as Page;
}

enablePatches();

function convert(base: JsonValue, recipe: (d: Record<string, unknown>) => void) {
  const [next, patches] = produceWithPatches<Record<string, unknown>>(base as Record<string, unknown>, recipe);
  return { next: next as JsonValue, ops: toJsonPatch(base, patches) };
}

describe("SRV-030 Immer patches to JSON Patch with test guards", () => {
  const base: JsonValue = { kind: "page", props: { padding: 8, title: "Login" }, children: ["a", "b", "c"] };

  test("replace is guarded by a test of the old value", () => {
    const { ops } = convert(base, (d) => {
      page(d).props.padding = 16;
    });
    expect(ops).toEqual([
      { op: "test", path: "/props/padding", value: 8 },
      { op: "replace", path: "/props/padding", value: 16 },
    ]);
  });

  test("array inserts test the array; remove tests the removed value; strict replay refuses an overwriting add", () => {
    const { ops } = convert(base, (d) => {
      const props = page(d).props;
      props.color = "red";
      delete props.title;
      page(d).children.push("d");
    });
    expect(ops).toContainEqual({ op: "add", path: "/props/color", value: "red" });
    expect(ops.filter((o) => o.op === "test").map((o) => o.path)).not.toContain("/props");
    expect(ops).toContainEqual({ op: "test", path: "/children", value: ["a", "b", "c"] });
    const theirs: JsonValue = {
      kind: "page",
      props: { padding: 8, title: "Login", color: "blue" },
      children: ["a", "b", "c"],
    };
    expect(() => applyPatch(theirs, ops, { strict: true })).toThrow(PatchError);
    expect(ops).toContainEqual({ op: "test", path: "/props/title", value: "Login" });
    expect(ops).toContainEqual({ op: "remove", path: "/props/title" });
  });

  test("array splices, pushes and nested changes round-trip onto the base", () => {
    const cases: ((d: Record<string, unknown>) => void)[] = [
      (d) => void page(d).children.splice(1, 1),
      (d) => void page(d).children.splice(0, 2, "x"),
      (d) => void page(d).children.push("d", "e"),
      (d) => void page(d).children.unshift("z"),
      (d) => {
        page(d).props.padding = 1;
        page(d).props.padding = 2;
        page(d).children = [];
      },
    ];
    for (const recipe of cases) {
      const { next, ops } = convert(base, recipe);
      expect(applyPatch(base, ops)).toEqual(next);
    }
  });

  test("a stale base never applies silently", () => {
    const { ops } = convert(base, (d) => {
      page(d).props.padding = 16;
    });
    const moved: JsonValue = { kind: "page", props: { padding: 12, title: "Login" }, children: ["a", "b", "c"] };
    expect(() => applyPatch(moved, ops)).toThrow(PatchError);
    const unrelated: JsonValue = { kind: "page", props: { padding: 8, title: "Login" }, children: ["a"] };
    expect(applyPatch(unrelated, ops)).toEqual({
      kind: "page",
      props: { padding: 16, title: "Login" },
      children: ["a"],
    });
  });

  test("pointers escape ~ and /, and the diff lists differing paths", () => {
    expect(pointer(["a/b", "c~d", 0])).toBe("/a~1b/c~0d/0");
    expect(diffPaths({ a: 1, b: { c: 2 } }, { a: 1, b: { c: 3 }, d: true }, 10)).toEqual(["/b/c", "/d"]);
  });
});
