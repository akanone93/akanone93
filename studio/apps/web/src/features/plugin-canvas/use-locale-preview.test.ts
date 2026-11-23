// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { act, renderHook } from "@testing-library/react";
import { usePluralSamples } from "./use-locale-preview.ts";

describe("the locale preview's plural samples", () => {
  test("stay one object across renders until a sample changes (#218)", () => {
    const { result, rerender } = renderHook(() => usePluralSamples());
    const first = result.current;
    rerender();
    // A new object on every render made every layer row new on every render of the canvas.
    expect(result.current).toBe(first);
    act(() => result.current.setSample("n1", 2));
    expect(result.current).not.toBe(first);
    expect(result.current.samples.get("n1")).toBe(2);
    expect(result.current.setSample).toBe(first.setSample);
  });
});
