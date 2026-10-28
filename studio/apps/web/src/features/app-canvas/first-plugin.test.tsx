// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";

afterEach(cleanup);

const path = `/o/${orgA}/apps/app-new`;
const empty = (over: Upstream = {}): Upstream => ({
  permissions: ["app.read", "app.manage"],
  creation: { calls: [] },
  summaries: [],
  pluginDocuments: {},
  appDocuments: [],
  ...over,
});

describe("the first plugin of a new app [STU-015]", () => {
  test("an app with no plugins says so, and New plugin creates the first one", async () => {
    const state = empty();
    renderApp(path, state);
    await screen.findByText("This app has no plugins yet.");
    fireEvent.click(screen.getByRole("button", { name: "New plugin" }));
    const dialog = await screen.findByRole("dialog", { name: "New plugin" });
    fireEvent.change(within(dialog).getByLabelText("Name"), { target: { value: "Loans" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.creation?.calls).toEqual(["plugin:app-new:loans:Loans"]));
    await waitFor(() => expect(screen.queryByText("This app has no plugins yet.") === null).toBe(true));
  });

  test("a refused plugin is reported and the dialog stays on the canvas", async () => {
    const state = empty({ creation: { calls: [], refuse: { plugin: "no" } } });
    renderApp(path, state);
    await screen.findByText("This app has no plugins yet.");
    fireEvent.click(screen.getByRole("button", { name: "New plugin" }));
    const dialog = await screen.findByRole("dialog", { name: "New plugin" });
    fireEvent.change(within(dialog).getByLabelText("Name"), { target: { value: "Loans" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await screen.findByText("The change could not be saved.");
  });
});
