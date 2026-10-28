// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgNew, renderApp, type Upstream } from "../../test-harness.tsx";

afterEach(cleanup);

const fill = async (name: string, key?: string) => {
  const dialog = await screen.findByRole("dialog", { name: "Create organisation" });
  fireEvent.change(within(dialog).getByLabelText("Name"), { target: { value: name } });
  if (key !== undefined) {
    fireEvent.change(within(dialog).getByLabelText("Key"), { target: { value: key } });
  }
  return dialog;
};
const submit = (dialog: HTMLElement) =>
  fireEvent.click(within(dialog).getByRole("button", { name: "Create organisation" }));

describe("creating an organisation from the empty state [STU-015]", () => {
  const empty = (over: Upstream = {}): Upstream => ({
    memberships: [],
    installationAdmin: true,
    creation: { calls: [] },
    ...over,
  });

  test("an administrator with no organisation creates the first and lands on its home", async () => {
    const state = empty();
    const r = renderApp("/", state);
    fireEvent.click(await screen.findByRole("button", { name: "Create organisation" }));
    const dialog = await fill("Acme Bank");
    expect((within(dialog).getByLabelText("Key") as HTMLInputElement).value).toBe("acme-bank");
    submit(dialog);
    await waitFor(() => expect(state.creation?.calls).toEqual(["organisation:acme-bank:Acme Bank"]));
    await waitFor(() => expect(r.router.state.location.pathname).toBe(`/o/${orgNew}`));
    await screen.findByRole("heading", { name: "Welcome to Acme Bank" });
  });

  test("a person who is not an installation administrator sees no button, only the ask", async () => {
    renderApp("/", empty({ installationAdmin: false }));
    await screen.findByText("Ask an administrator to invite you to an organisation.");
    expect(screen.queryByRole("button", { name: "Create organisation" }) === null).toBe(true);
  });

  test("an empty name and a malformed key are refused before any request, with the reason", async () => {
    const state = empty();
    renderApp("/", state);
    fireEvent.click(await screen.findByRole("button", { name: "Create organisation" }));
    const dialog = await fill("   ", "Not A Key");
    submit(dialog);
    await within(dialog).findByText("Enter a name of 1 to 200 characters.");
    within(dialog).getByText(/Use 1 to 64 lower-case letters/);
    expect(state.creation?.calls).toEqual([]);
  });

  test("a name over 200 bytes is refused like the server would", async () => {
    const state = empty();
    renderApp("/", state);
    fireEvent.click(await screen.findByRole("button", { name: "Create organisation" }));
    submit(await fill("ä".repeat(101), "acme"));
    await screen.findByText("Enter a name of 1 to 200 characters.");
    expect(state.creation?.calls).toEqual([]);
  });

  test("the server's refusal is shown inside the dialog and nothing is switched", async () => {
    const state = empty({
      creation: { calls: [], refuse: { organisation: "the installation is at its organisation limit" } },
    });
    const r = renderApp("/", state);
    fireEvent.click(await screen.findByRole("button", { name: "Create organisation" }));
    const dialog = await fill("Acme", "acme");
    submit(dialog);
    const alert = await within(dialog).findByRole("alert");
    expect(alert.textContent).toBe("the installation is at its organisation limit");
    expect(r.router.state.location.pathname).toBe("/");
  });
});

describe("creating an organisation from the top bar [STU-015]", () => {
  test("an administrator opens the dialog beside the switcher and switches to the new organisation", async () => {
    const state: Upstream = { installationAdmin: true, creation: { calls: [] } };
    const r = renderApp(`/o/${"0192f000-0000-7000-8000-00000000000a"}`, state);
    fireEvent.click(await screen.findByRole("button", { name: "New organisation" }));
    submit(await fill("Gamma", "gamma"));
    await waitFor(() => expect(state.creation?.calls).toEqual(["organisation:gamma:Gamma"]));
    await waitFor(() => expect(r.router.state.location.pathname).toBe(`/o/${orgNew}`));
    expect((await screen.findByRole("combobox", { name: "Organisation" })).textContent).toContain("Gamma");
  });

  test("the button is hidden from a person who is not an installation administrator", async () => {
    renderApp("/o/0192f000-0000-7000-8000-00000000000a", { installationAdmin: false });
    await screen.findByRole("combobox", { name: "Organisation" });
    expect(screen.queryByRole("button", { name: "New organisation" }) === null).toBe(true);
  });
});
