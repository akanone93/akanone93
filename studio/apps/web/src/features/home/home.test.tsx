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
const fill = (dialog: HTMLElement, name: string, key?: string) => {
  fireEvent.change(within(dialog).getByLabelText("Name"), { target: { value: name } });
  if (key !== undefined) {
    fireEvent.change(within(dialog).getByLabelText("Key"), { target: { value: key } });
  }
};
const submit = (dialog: HTMLElement) => fireEvent.click(within(dialog).getByRole("button", { name: "Create app" }));
const manager = (over: Upstream = {}): Upstream => ({
  permissions: ["app.read", "app.manage"],
  creation: { calls: [] },
  ...over,
});

describe("the organisation's apps and creating one [STU-015]", () => {
  test("the apps are listed and link to their canvas", async () => {
    renderApp(home, manager({ creation: { calls: [], apps: [{ id: "a1", key: "bank", name: "Bank" }] } }));
    const link = await screen.findByRole("link", { name: "Open Bank" });
    expect(link.getAttribute("href")).toBe(`/o/${orgA}/apps/a1`);
  });

  test("a manager creates an app from the empty state and lands on its canvas", async () => {
    const state = manager({ creation: { calls: [], apps: [] } });
    const r = renderApp(home, state);
    await screen.findByText("This organisation has no apps yet.");
    const dialog = await open();
    fill(dialog, "Plux Bank");
    expect((within(dialog).getByLabelText("Key") as HTMLInputElement).value).toBe("plux-bank");
    submit(dialog);
    await waitFor(() => expect(state.creation?.calls).toEqual([`app:${orgA}:plux-bank:Plux Bank`]));
    await waitFor(() => expect(r.router.state.location.pathname).toBe(`/o/${orgA}/apps/app-plux-bank`));
  });

  test("with apps listed the create button stays in the header, and an empty name is refused", async () => {
    const state = manager();
    renderApp(home, state);
    await screen.findByRole("link", { name: "Open Shop" });
    submit(await open());
    await within(await screen.findByRole("dialog", { name: "Create app" })).findByText(
      "Enter a name of 1 to 200 characters.",
    );
    expect(state.creation?.calls).toEqual([]);
  });

  test("an invalid key is refused before any request", async () => {
    const state = manager();
    renderApp(home, state);
    const dialog = await open();
    fill(dialog, "Shop", "-bad-");
    submit(dialog);
    await within(dialog).findByText(/Use 1 to 64 lower-case letters/);
    expect(state.creation?.calls).toEqual([]);
  });

  test("the server's refusal, such as a quota, is shown in the dialog", async () => {
    const state = manager({ creation: { calls: [], refuse: { app: "the organisation has reached its app limit" } } });
    const r = renderApp(home, state);
    const dialog = await open();
    fill(dialog, "Extra", "extra");
    submit(dialog);
    expect((await within(dialog).findByRole("alert")).textContent).toBe("the organisation has reached its app limit");
    expect(r.router.state.location.pathname).toBe(home);
  });

  test("without app.manage there is no create button, and the empty state says whom to ask", async () => {
    renderApp(home, { permissions: ["app.read"], creation: { calls: [], apps: [] } });
    await screen.findByText("Ask an administrator to create one.");
    expect(screen.queryByRole("button", { name: "Create app" }) === null).toBe(true);
  });
});
