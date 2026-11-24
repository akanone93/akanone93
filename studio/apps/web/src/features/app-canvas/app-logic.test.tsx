// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import type { CanvasSurface } from "../plugin-canvas/surface.ts";
import type { ValidatorClient } from "../problems/validator-client.ts";
import { appHandlers } from "./app-logic.tsx";

afterEach(cleanup);

const surface = (): CanvasSurface => ({
  layoutScreens: async () => [],
  setViewport: () => undefined,
  pan: () => undefined,
  zoomAbout: () => undefined,
  viewport: { x: 0, y: 0, zoom: 1 },
  dispose: () => undefined,
});
const validator = (): ValidatorClient => ({
  load: async () => [],
  validatePage: async () => [],
  dispose: () => undefined,
});

describe("the app's own logic on the app canvas (#284)", () => {
  test("the app's handlers: the lifecycle ones, each declared or handled host event, each handled data source", () => {
    const keys = appHandlers({
      hostEvents: [{ name: "paid" }, { name: "signedOut" }],
      triggers: { hostEvents: { legacy: { steps: [] } }, dataSources: { feed: { steps: [] } } },
    }).map((h) => `${h.key}=${h.pointer.join("/")}`);
    expect(keys).toEqual([
      "onAppResume=triggers/onAppResume",
      "onAppPause=triggers/onAppPause",
      "onPushOpened=triggers/onPushOpened",
      "onError=triggers/onError",
      "hostEvents.legacy=triggers/hostEvents/legacy",
      "hostEvents.paid=triggers/hostEvents/paid",
      "hostEvents.signedOut=triggers/hostEvents/signedOut",
      "dataSources.feed=triggers/dataSources/feed",
    ]);
  });

  test("the Overview opens the graph editor on the app's resume handler; a step added is saved in app.json", async () => {
    const sent: NonNullable<Parameters<typeof renderApp>[1]>["sent"] = [];
    renderApp(`/o/${orgA}/apps/app1`, {
      surface: () => surface(),
      summaries: [{ id: "loans", key: "loans", name: "Loans" }],
      permissions: ["app.read", "app.manage", "plugin.read", "plugin.write"],
      appDocuments: [
        { kind: "app", path: "app.json", json: { schemaVersion: "1.0.0", kind: "app", entryRoute: "home" } },
      ],
      sent,
      validator,
    });
    fireEvent.click(await screen.findByRole("tab", { name: /Overview/ }, { timeout: 5000 }));
    fireEvent.click(await screen.findByRole("button", { name: "App logic…" }));
    expect(await screen.findByText(/no steps yet/)).toBeTruthy();
    const catalogue = screen.getByRole("region", { name: "Actions" });
    fireEvent.change(within(catalogue).getByRole("textbox", { name: "Search actions" }), {
      target: { value: "kvget" },
    });
    fireEvent.click(within(catalogue).getByRole("button", { name: "Add kvGet to the graph" }));
    await waitFor(
      () =>
        expect(
          sent.some(
            (s) => s.path === "app.json" && s.ops.some((o) => o.op === "add" && o.path.startsWith("/triggers")),
          ),
        ).toBe(true),
      { timeout: 5000 },
    );
  });
});
