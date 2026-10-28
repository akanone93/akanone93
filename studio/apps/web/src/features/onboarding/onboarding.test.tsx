// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";

afterEach(cleanup);

const home = `/o/${orgA}`;
const samples = [
  { id: "starter", key: "starter", name: "Starter", description: "The smallest app that runs.", size: 4096 },
  { id: "plux-bank", key: "plux-bank", name: "Plux Bank", description: "A banking app.", size: 8192 },
  { id: "plux-express", key: "plux-express", name: "Plux Express", description: "A delivery app.", size: 8192 },
];
const fresh = [
  { key: "create_app", done: false },
  { key: "edit_screen", done: false },
  { key: "pair_device", done: false },
  { key: "publish_release", done: false },
  { key: "invite_member", done: false },
];
const manager = (over: Upstream = {}): Upstream => ({
  permissions: ["app.read", "app.manage"],
  creation: { calls: [], apps: [] },
  projects: { calls: [], samples, onboarding: fresh },
  ...over,
});
const checklist = () => screen.findByRole("region", { name: "Get started" }, { timeout: 10000 });

describe("the first-run checklist [STU-015]", () => {
  test("a new organisation sees the steps in the order create, pair, edit, publish, invite, all still to do", async () => {
    renderApp(home, manager());
    const region = await checklist();
    const items = within(within(region).getByRole("list", { name: "Checklist" })).getAllByRole("listitem");
    expect(items.map((li) => li.textContent?.replace(/\s+/g, " ").trim().slice(0, 30))).toEqual([
      "To do Create or import an app ",
      "To do Pair a device Connect a",
      "To do Edit a screen Change a s",
      "To do Publish a release Publish",
      "To do Invite a teammate Add a ",
    ]);
    expect(within(region).getByRole("meter", { name: "Getting started progress" }).getAttribute("aria-valuetext")).toBe(
      "0 of 5 steps",
    );
  });

  test("progress comes from the server: steps it reports done are marked, and the checklist goes away when all are", async () => {
    const steps = fresh.map((s) => ({ ...s, done: s.key === "create_app" || s.key === "edit_screen" }));
    renderApp(home, manager({ projects: { calls: [], samples, onboarding: steps } }));
    const region = await checklist();
    expect(within(region).getByRole("meter", { name: "Getting started progress" }).getAttribute("aria-valuetext")).toBe(
      "2 of 5 steps",
    );
    expect(within(region).getAllByText("Done").length).toBe(2);
    cleanup();
    renderApp(
      home,
      manager({ projects: { calls: [], samples, onboarding: fresh.map((s) => ({ ...s, done: true })) } }),
    );
    await screen.findByRole("heading", { name: "Welcome to Alpha Org" }, { timeout: 10000 });
    expect(screen.queryByRole("region", { name: "Get started" }) === null).toBe(true);
  });

  test("a step cannot be ticked by hand: the done steps have no control", async () => {
    renderApp(home, manager());
    const region = await checklist();
    expect(within(region).queryAllByRole("checkbox").length).toBe(0);
  });

  test("dismissing stores the choice in the preferences, hides the checklist and leaves a way to show it again", async () => {
    const updates: NonNullable<Upstream["updates"]> = [];
    renderApp(home, manager({ updates, stored: { theme: "", language: "", density: "" } }));
    const region = await checklist();
    fireEvent.click(within(region).getByRole("button", { name: "Dismiss checklist" }));
    await waitFor(() => expect(updates.at(-1)?.paths).toEqual(["onboarding_dismissed"]));
    expect(updates.at(-1)?.onboardingDismissed).toBe(true);
    fireEvent.click(await screen.findByRole("button", { name: "Show getting started" }));
    await waitFor(() => expect(updates.at(-1)?.onboardingDismissed).toBe(false));
    expect(await checklist()).toBeTruthy();
  });

  test("a dismissed checklist stays dismissed after a reload", async () => {
    renderApp(home, manager({ stored: { theme: "", language: "", density: "", onboardingDismissed: true } }));
    await screen.findByRole("button", { name: "Show getting started" }, { timeout: 10000 });
    expect(screen.queryByRole("region", { name: "Get started" }) === null).toBe(true);
  });

  test("the first step opens the new-app wizard", async () => {
    renderApp(home, manager());
    const region = await checklist();
    fireEvent.click(within(region).getByRole("button", { name: "Start a new app" }));
    expect(await screen.findByRole("dialog", { name: "Create app" })).toBeTruthy();
  });

  test("the later steps link to the first app once there is one", async () => {
    const r = renderApp(home, manager({ creation: { calls: [], apps: [{ id: "a1", key: "bank", name: "Bank" }] } }));
    const region = await checklist();
    const links = await within(region).findAllByRole("link", { name: "Continue in Bank" });
    expect(links.length).toBe(4);
    expect(links[0]?.getAttribute("href")).toBe(`${home}/apps/a1`);
    expect(r.router.state.location.pathname).toBe(home);
  });

  test("without app.manage the steps are shown but there is nothing to create or import", async () => {
    renderApp(home, manager({ permissions: ["app.read"] }));
    const region = await checklist();
    expect(within(region).queryByRole("button", { name: "Start a new app" }) === null).toBe(true);
    expect(within(region).queryByText("Sample apps") === null).toBe(true);
  });
});

describe("importing a sample app [STU-015]", () => {
  test("the server's sample apps are listed and one press imports one as the person's own copy", async () => {
    const state = manager();
    const r = renderApp(home, state);
    const region = await checklist();
    const list = await within(region).findByRole("list", { name: "Sample apps to import" });
    expect(
      within(list)
        .getAllByRole("button")
        .map((b) => b.textContent),
    ).toEqual(["Import Starter", "Import Plux Bank", "Import Plux Express"]);
    fireEvent.click(within(list).getByRole("button", { name: "Import Plux Bank" }));
    await waitFor(() => expect(state.projects?.calls).toEqual([`sample:${orgA}:plux-bank::`]));
    await waitFor(() => expect(r.router.state.location.pathname).toBe(`${home}/apps/app-plux-bank`));
  });

  test("the server's refusal, such as a key that is taken, is shown and the checklist stays", async () => {
    const state = manager({
      projects: { calls: [], samples, onboarding: fresh, refuse: { sample: "an app with the key plux-bank exists" } },
    });
    const r = renderApp(home, state);
    const region = await checklist();
    fireEvent.click(await within(region).findByRole("button", { name: "Import Plux Bank" }));
    expect((await within(region).findByRole("alert")).textContent).toBe("an app with the key plux-bank exists");
    expect(r.router.state.location.pathname).toBe(home);
  });
});
