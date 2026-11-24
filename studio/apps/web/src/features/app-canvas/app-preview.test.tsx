// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";

afterEach(cleanup);

/** Picks an option of a select as a pointer does (the select ignores a bare click while it opens). */
async function choose(option: HTMLElement): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 300));
  fireEvent.pointerDown(option);
  fireEvent.mouseDown(option);
  fireEvent.pointerUp(option);
  fireEvent.mouseUp(option);
  fireEvent.click(option);
}

const surface = (): CanvasSurface => ({
  layoutScreens: async () => [],
  setViewport: () => undefined,
  pan: () => undefined,
  zoomAbout: () => undefined,
  viewport: { x: 0, y: 0, zoom: 1 },
  dispose: () => undefined,
});

describe("the preview switches on the app canvas (#286)", () => {
  test("the app canvas sets the person's preview of the app, the one every plugin canvas of it draws in", async () => {
    const updates: { paths: string[]; preview?: Record<string, unknown> }[] = [];
    renderApp(`/o/${orgA}/apps/app1`, {
      surface: () => surface(),
      summaries: [{ id: "loans", key: "loans", name: "Loans" }],
      permissions: ["app.read", "plugin.read"],
      updates: updates as never,
    });
    fireEvent.click(await screen.findByRole("button", { name: "Preview" }, { timeout: 5000 }));
    const dialog = await screen.findByRole("dialog", { name: "Preview" });
    fireEvent.click(within(dialog).getByRole("combobox", { name: "Theme" }));
    await choose(await screen.findByRole("option", { name: "Dark" }));
    await waitFor(() => expect(updates.some((u) => u.paths.includes("preview"))).toBe(true));
    const preview = updates.find((u) => u.paths.includes("preview"))?.preview;
    expect(Object.keys(preview ?? {})).toEqual(["app1"]);
  });
});
