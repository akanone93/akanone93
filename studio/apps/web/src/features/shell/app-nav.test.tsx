// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp } from "../../test-harness.tsx";
import { appNavEntries, visibleAppNav } from "./app-nav.tsx";

afterEach(() => {
  cleanup();
  // The remembered app lives in the browser's storage; each test starts without one.
  globalThis.localStorage?.clear();
});

const base = `/o/${orgA}/apps/app1`;
const reader = ["app.read"];

describe("[STU-100] to [STU-116] navigation between the areas of an app", () => {
  test("a reader sees every area their permissions open, as links to the area's route", async () => {
    renderApp(`${base}/releases`, { permissions: reader });
    const nav = await screen.findByRole("navigation", { name: "App areas" }, { timeout: 20_000 });
    const hrefs = Object.fromEntries(
      within(nav)
        .getAllByRole("link")
        .map((a) => [a.textContent, a.getAttribute("href")]),
    );
    expect(hrefs).toEqual({
      Canvas: base,
      Releases: `${base}/releases`,
      Functions: `${base}/functions`,
      Data: `${base}/data`,
      Localisation: `${base}/localisation`,
      Design: `${base}/design`,
      Analytics: `${base}/analytics`,
      Tests: `${base}/tests`,
      Experiments: `${base}/experiments`,
      Approvals: `/o/${orgA}/approvals?app=app1`,
      Devices: `${base}/devices`,
      Security: `${base}/security`,
      Settings: `${base}/settings`,
    });
  }, 40_000);

  test("entries the person's permissions do not allow are not shown", async () => {
    renderApp(`${base}/releases`, { permissions: [...reader, "release.publish"] });
    const nav = await screen.findByRole("navigation", { name: "App areas" }, { timeout: 20_000 });
    expect(within(nav).getByRole("link", { name: "Publish" }).getAttribute("href")).toBe(`${base}/publish`);
    expect(within(nav).getByRole("link", { name: "Settings" }).getAttribute("href")).toBe(`${base}/settings`);
    cleanup();
    renderApp(`${base}/releases`, { permissions: reader });
    const plain = await screen.findByRole("navigation", { name: "App areas" }, { timeout: 20_000 });
    expect(within(plain).queryByRole("link", { name: "Publish" }) === null).toBe(true);
  }, 40_000);

  test("the links take the keyboard focus and open their area", async () => {
    const { router } = renderApp(`${base}/releases`, { permissions: reader });
    const nav = await screen.findByRole("navigation", { name: "App areas" }, { timeout: 20_000 });
    const link = within(nav).getByRole("link", { name: "Functions" });
    link.focus();
    expect(document.activeElement).toBe(link);
    expect(link.tabIndex).toBeGreaterThanOrEqual(0);
    fireEvent.click(link);
    await waitFor(() => expect(router.state.location.pathname).toBe(`${base}/functions`));
    await waitFor(() =>
      expect(
        within(screen.getByRole("navigation", { name: "App areas" }))
          .getByRole("link", { name: "Functions" })
          .getAttribute("aria-current"),
      ).toBe("page"),
    );
  }, 40_000);

  test("the entries keep the permission each route guard needs", () => {
    expect(visibleAppNav([]).length).toBe(0);
    expect(visibleAppNav(["release.publish"]).map((e) => e.id)).toEqual(["publish"]);
    expect(appNavEntries.every((e) => e.permission !== "")).toBe(true);
  });

  test("[#202] on a page without an app in its address, the areas stay for the app last opened", async () => {
    const apps = [
      { id: "app1", key: "shop", name: "Shop" },
      { id: "app2", key: "cafe", name: "Cafe" },
    ];
    // Open the second app once: it becomes the organisation's remembered app, ahead of the first one.
    renderApp(`/o/${orgA}/apps/app2/releases`, { permissions: reader, creation: { apps, calls: [] } });
    await screen.findByRole("navigation", { name: "App areas" }, { timeout: 20_000 });
    cleanup();
    renderApp(`/o/${orgA}`, { permissions: reader, creation: { apps, calls: [] } });
    const nav = await screen.findByRole("navigation", { name: "App areas" }, { timeout: 20_000 });
    expect(within(nav).getByRole("link", { name: "Settings" }).getAttribute("href")).toBe(
      `/o/${orgA}/apps/app2/settings`,
    );
    expect(within(nav).getByRole("button", { name: "App: Cafe. Switch app" })).toBeTruthy();
  }, 40_000);

  test("[#202] without a remembered app the areas are the organisation's first app's", async () => {
    renderApp(`/o/${orgA}`, { permissions: reader });
    const nav = await screen.findByRole("navigation", { name: "App areas" }, { timeout: 20_000 });
    expect(within(nav).getByRole("link", { name: "Canvas" }).getAttribute("href")).toBe(base);
  }, 40_000);

  test("[#202] the app's own Approvals entry keeps the app's areas, and only one approvals page is current", async () => {
    const { router } = renderApp(`${base}/releases`, { permissions: reader });
    const nav = await screen.findByRole("navigation", { name: "App areas" }, { timeout: 20_000 });
    fireEvent.click(within(nav).getByRole("link", { name: "Approvals" }));
    await waitFor(() => expect(router.state.location.pathname).toBe(`/o/${orgA}/approvals`));
    const after = await screen.findByRole("navigation", { name: "App areas" }, { timeout: 20_000 });
    expect(within(after).getByRole("link", { name: "Approvals" }).getAttribute("aria-current")).toBe("page");
    expect(within(after).getByRole("link", { name: "Releases" }).getAttribute("href")).toBe(`${base}/releases`);
    const studio = screen.getByRole("navigation", { name: "Studio navigation" });
    const current = within(studio)
      .getAllByRole("link")
      .filter((a) => a.hasAttribute("data-active"))
      .map((a) => a.textContent);
    expect(current).toEqual(["Inbox"]);
  }, 40_000);

  test("[#202] with no app the person may open there is no app navigation", async () => {
    const { session } = renderApp(`/o/${orgA}`, { permissions: reader, creation: { apps: [], calls: [] } });
    await screen.findByRole("banner");
    // Once the list of apps has arrived, the navigation has had everything it decides by.
    await waitFor(() => expect(session.queryClient.getQueryState(["nav-apps", orgA])?.status).toBe("success"));
    expect(screen.queryByRole("navigation", { name: "App areas" }) === null).toBe(true);
  }, 40_000);
});
