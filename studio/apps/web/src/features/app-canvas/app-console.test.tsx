// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { create } from "@bufbuild/protobuf";
import { DevEventSchema, Severity } from "@plux/api";
import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";

afterEach(cleanup);

const surface = (): CanvasSurface => ({
  layoutScreens: async () => [],
  setViewport: () => undefined,
  pan: () => undefined,
  zoomAbout: () => undefined,
  viewport: { x: 0, y: 0, zoom: 1 },
  dispose: () => undefined,
});

describe("the developer console on the app canvas (#282)", () => {
  test("it opens on the app's problems and reads the phone's logs, with the link's state shown once", async () => {
    const line = create(DevEventSchema, {
      sequence: 1n,
      deviceId: "d1",
      payload: { case: "log", value: { source: "app", message: "hello from the phone" } },
    });
    renderApp(`/o/${orgA}/apps/app1`, {
      surface: () => surface(),
      summaries: [{ id: "loans", key: "loans", name: "Loans" }],
      permissions: ["app.read", "plugin.read", "devsession.create"],
      devSession: { devices: [{ id: "d1", name: "Pixel 9", connected: true }], events: [line] },
      appDiagnostics: [
        {
          code: "PLX-1103",
          severity: Severity.ERROR,
          file: "plugins/loans/pages/home.page.json",
          path: "/route",
          message: "a problem the app's check found",
        },
      ],
    });
    expect(await screen.findByText("Live · 1 device")).toBeTruthy();
    fireEvent.click(await screen.findByRole("button", { name: /Developer console/ }));
    const bay = screen.getByRole("region", { name: /Developer console/ });
    expect(await within(bay).findByText("a problem the app's check found")).toBeTruthy();
    fireEvent.click(await within(bay).findByRole("tab", { name: /^Logs/ }));
    expect(await within(bay).findByText(/hello from the phone/)).toBeTruthy();
    // The canvas's link bar shows the state; the console does not repeat it.
    expect(screen.getAllByText("Live · 1 device")).toHaveLength(1);
  });
});
