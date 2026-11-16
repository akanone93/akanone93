// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import { ViewMap } from "./view-map.tsx";

afterEach(() => {
  cleanup();
  globalThis.localStorage?.clear();
});

const labels = { label: "Map of the canvas", hide: "Hide the map", show: "Show the map" };

/** A surface whose pan moves its view as the real one does: the content by (dx, dy), the origin by the opposite. */
function rig() {
  const pans: [number, number][] = [];
  const viewport = { x: 0, y: 0, zoom: 1 };
  const surface = {
    viewport,
    pan: (dx: number, dy: number) => {
      pans.push([dx, dy]);
      viewport.x -= dx / viewport.zoom;
      viewport.y -= dy / viewport.zoom;
    },
  } as unknown as CanvasSurface;
  const canvas = { clientWidth: 1000, clientHeight: 600 } as unknown as HTMLElement;
  let views = 0;
  render(
    <ViewMap
      surface={() => surface}
      canvas={() => canvas}
      items={[
        { id: "home", l: 0, t: 0, r: 390, b: 844 },
        { id: "form", l: 3000, t: 0, r: 3390, b: 844 },
      ]}
      labels={labels}
      onView={() => {
        views += 1;
      }}
    />,
  );
  return { pans, viewport, views: () => views };
}

describe("the view map above the zoom (C5)", () => {
  test("the arrow keys move the view by a tenth of its size, Shift by half", async () => {
    const r = rig();
    const map = screen.getByRole("button", { name: labels.label });
    await waitFor(() => expect(map.querySelector(".view-map-view") !== null).toBe(true));
    fireEvent.keyDown(map, { key: "ArrowRight" });
    expect(r.viewport.x).toBeCloseTo(100, 6);
    fireEvent.keyDown(map, { key: "ArrowDown", shiftKey: true });
    expect(r.viewport.y).toBeCloseTo(300, 6);
    expect(r.views()).toBe(2);
  });

  test("a press away from the frame brings the view there", async () => {
    const r = rig();
    const map = screen.getByRole("button", { name: labels.label });
    await waitFor(() => expect(map.querySelector(".view-map-view") !== null).toBe(true));
    // The far right of the map is the second screen, well outside the view.
    map.getBoundingClientRect = () => ({ left: 0, top: 0, width: 184, height: 116 }) as DOMRect;
    fireEvent.pointerDown(map, { button: 0, clientX: 180, clientY: 58, pointerId: 1 });
    expect(r.pans.length).toBe(1);
    expect(r.viewport.x).toBeGreaterThan(2000);
  });

  test("it folds to a button and comes back as it was left", async () => {
    rig();
    fireEvent.click(screen.getByRole("button", { name: labels.hide }));
    expect(screen.queryByRole("button", { name: labels.label })).toBeNull();
    cleanup();
    rig();
    fireEvent.click(screen.getByRole("button", { name: labels.show }));
    expect(screen.getByRole("button", { name: labels.label })).toBeDefined();
  });
});
