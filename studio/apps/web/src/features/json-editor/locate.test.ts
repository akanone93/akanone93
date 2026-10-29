// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { lineColumn, locate, pointerSegments } from "./locate.ts";
import { mergeInto } from "./merge.ts";

const text = '{\n  "a": [1, {"b~c": "x"}],\n  "d/e": true\n}\n';
const at = (pointer: string) => {
  const span = locate(text, pointer);
  return span === undefined ? undefined : text.slice(span.start, span.end);
};

describe("[STU-064] locating a validator pointer in the editor text", () => {
  test("finds the value a pointer names, through objects and arrays and escapes", () => {
    expect(at("/a/0")).toBe("1");
    expect(at("/a/1/b~0c")).toBe('"x"');
    expect(at("/d~1e")).toBe("true");
    expect(at("/a")).toBe('[1, {"b~c": "x"}]');
    expect(at("")?.startsWith("{")).toBe(true);
  });

  test("falls back to the nearest ancestor that exists, and refuses what is not a pointer or not JSON", () => {
    expect(at("/a/9/zzz")).toBe('[1, {"b~c": "x"}]');
    expect(at("/missing")?.startsWith("{")).toBe(true);
    expect(locate(text, "no-slash")).toBeUndefined();
    expect(pointerSegments("x")).toBeUndefined();
    expect(locate("{ not json", "/a")).toBeUndefined();
    expect(locate('{"a": [1,', "/a/0")).toBeDefined();
    expect(locate("", "")).toBeUndefined();
  });

  test("converts offsets to 1-based lines and columns", () => {
    expect(lineColumn(text, 0)).toEqual({ line: 1, column: 1 });
    expect(lineColumn(text, text.indexOf('"a"'))).toEqual({ line: 2, column: 3 });
    expect(lineColumn(text, text.indexOf("true"))).toEqual({ line: 3, column: 10 });
  });
});

describe("[STU-064] applying edited JSON as a minimal change", () => {
  test("keeps equal members, changes, adds and removes the rest", () => {
    const draft: Record<string, unknown> = { keep: { deep: [1, 2], gone: 1 }, list: [1], drop: true, same: "s" };
    const keep = draft["keep"];
    mergeInto(draft, { keep: { deep: [1, 2], added: 2 }, list: [1, 2], same: "s", fresh: null });
    expect(draft).toEqual({ keep: { deep: [1, 2], added: 2 }, list: [1, 2], same: "s", fresh: null });
    expect(draft["keep"]).toBe(keep);
  });
});
