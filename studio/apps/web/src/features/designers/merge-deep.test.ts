// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { mergeDeep } from "./merge-deep.ts";

describe("mergeDeep", () => {
  test("merges objects inside arrays and appends and truncates", () => {
    const draft: Record<string, unknown> = { a: [{ x: 1, y: 2 }, { x: 3 }], keep: true, gone: 1 };
    mergeDeep(draft, { a: [{ x: 1, y: 5 }, { x: 3 }, { x: 4 }], keep: true });
    expect(draft).toEqual({ a: [{ x: 1, y: 5 }, { x: 3 }, { x: 4 }], keep: true });
    mergeDeep(draft, { a: [{ x: 1, y: 5 }], keep: true });
    expect(draft["a"]).toEqual([{ x: 1, y: 5 }]);
  });
  test("replaces scalars in nested lists and equal values stay untouched", () => {
    const inner = { n: [1, 2] };
    const draft: Record<string, unknown> = { l: [[1, 2], 3], o: inner };
    mergeDeep(draft, { l: [[1, 9], 3], o: { n: [1, 2] } });
    expect(draft["l"]).toEqual([[1, 9], 3]);
    expect(draft["o"]).toBe(inner);
  });
});
