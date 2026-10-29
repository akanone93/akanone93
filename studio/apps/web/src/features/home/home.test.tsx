// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";

afterEach(cleanup);

const home = `/o/${orgA}`;
const open = async () => {
  fireEvent.click(await screen.findByRole("button", { name: "Create app" }));
  return screen.findByRole("dialog", { name: "Create app" });
};
const manager = (over: Upstream = {}): Upstream => ({
  permissions: ["app.read", "app.manage"],
  creation: { calls: [] },
  projects: { calls: [], archives: [] },
  ...over,
});

describe("the organisation's apps and the new-app wizard [STU-024]", () => {
  test("the apps are listed and link to their canvas", async () => {
    renderApp(home, manager({ creation: { calls: [], apps: [{ id: "a1", key: "bank", name: "Bank" }] } }));
    const link = await screen.findByRole("link", { name: "Open Bank" });
    expect(link.getAttribute("href")).toBe(`/o/${orgA}/apps/a1`);
  });

  test("a manager creates an app from the empty state with the wizard and lands on its canvas", async () => {
    const state = manager({ creation: { calls: [], apps: [] } });
    const r = renderApp(home, state);
    await screen.findByText("This organisation has no apps yet.");
    const dialog = await open();
    fireEvent.click(within(dialog).getByRole("button", { name: "Next" }));
    fireEvent.change(within(dialog).getByLabelText("Name"), { target: { value: "Plux Bank" } });
    expect((within(dialog).getByLabelText("Key") as HTMLInputElement).value).toBe("plux-bank");
    for (let i = 0; i < 4; i += 1) {
      fireEvent.click(within(dialog).getByRole("button", { name: "Next" }));
    }
    fireEvent.click(within(dialog).getByRole("button", { name: "Create app" }));
    await waitFor(() => expect(state.projects?.calls.length).toBe(1));
    expect(state.projects?.calls[0]?.startsWith(`import:${orgA}:plux-bank:Plux Bank:`)).toBe(true);
    fireEvent.click(await within(dialog).findByRole("button", { name: "Open the app" }));
    await waitFor(() => expect(r.router.state.location.pathname).toBe(`/o/${orgA}/apps/app-plux-bank`));
  });

  test("with apps listed the create button stays in the header and opens the wizard", async () => {
    renderApp(home, manager());
    await screen.findByRole("link", { name: "Open Shop" });
    const dialog = await open();
    expect(within(dialog).getByRole("heading", { name: "Step 1 of 7: Start from" })).toBeTruthy();
  });

  test("without app.manage there is no create button, and the empty state says whom to ask", async () => {
    renderApp(home, { permissions: ["app.read"], creation: { calls: [], apps: [] } });
    await screen.findByText("Ask an administrator to create one.");
    expect(screen.queryByRole("button", { name: "Create app" }) === null).toBe(true);
  });
});
