// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, render, renderHook, screen } from "@testing-library/react";
import { CanvasLoading, type FirstContent, useFirstLoad } from "./canvas-loading.tsx";

afterEach(cleanup);

describe("a canvas's first load (#211)", () => {
  test("lasts until the content is ready once, and never comes back on a later refresh", () => {
    const pending: FirstContent = { failed: false, loaded: false, items: 0, drawn: false };
    const { result, rerender } = renderHook((c: FirstContent) => useFirstLoad(c), { initialProps: pending });
    expect(result.current).toBe(true);
    // Loaded but not yet laid out: still loading; laid out, but waiting for the links: still loading.
    rerender({ ...pending, loaded: true, items: 3 });
    expect(result.current).toBe(true);
    rerender({ ...pending, loaded: true, items: 3, drawn: true, linked: false });
    expect(result.current).toBe(true);
    rerender({ ...pending, loaded: true, items: 3, drawn: true });
    expect(result.current).toBe(false);
    // A refetch makes the content pending again: the canvas keeps what it shows, without the loading state.
    rerender(pending);
    expect(result.current).toBe(false);
  });

  test("is over at once when there is nothing to draw, or the load failed", () => {
    const none = renderHook(() => useFirstLoad({ failed: false, loaded: true, items: 0, drawn: false }));
    expect(none.result.current).toBe(false);
    const failed = renderHook(() => useFirstLoad({ failed: true, loaded: false, items: 0, drawn: false }));
    expect(failed.result.current).toBe(false);
  });

  test("is a status the pointer passes through, with only the label announced", () => {
    const { rerender } = render(<CanvasLoading label="Loading screens" shown />);
    const status = screen.getByRole("status");
    expect(status.textContent).toBe("Loading screens");
    expect(status.querySelector("svg")?.getAttribute("aria-hidden")).toBe("true");
    rerender(<CanvasLoading label="Loading screens" shown={false} />);
    expect(screen.queryByRole("status")).toBeNull();
  });
});
