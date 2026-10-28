// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, beforeAll, describe, expect, setDefaultTimeout, test } from "bun:test";
import { limitDefault } from "@plux/schema";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import type { AppsGridApp, AppsGridFixture } from "../../test-apps.ts";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";

setDefaultTimeout(30_000);
afterEach(cleanup);

// happy-dom never reports an element as visible; the cards read their images once they are on screen.
beforeAll(() => {
  globalThis.IntersectionObserver = class {
    readonly #callback: IntersectionObserverCallback;
    constructor(callback: IntersectionObserverCallback) {
      this.#callback = callback;
    }
    observe(target: Element): void {
      queueMicrotask(() =>
        this.#callback([{ isIntersecting: true, target } as IntersectionObserverEntry], this as never),
      );
    }
    unobserve(): void {}
    disconnect(): void {}
    takeRecords(): IntersectionObserverEntry[] {
      return [];
    }
  } as unknown as typeof IntersectionObserver;
});

const home = `/o/${orgA}`;
const wait = { timeout: 10_000 };
const teams = [
  { id: "t1", key: "web", name: "Web" },
  { id: "t2", key: "pay", name: "Payments" },
];

const bank: AppsGridApp = {
  id: "a-bank",
  key: "bank",
  name: "Bank",
  description: "Mobile banking for retail customers.",
  owningTeamId: "t2",
  tags: ["finance"],
  lastPublishedAt: 1_700_000_000,
  releases: [
    { environmentKey: "production", liveSequence: 3 },
    { environmentKey: "staging", liveSequence: 0 },
  ],
  activeDevices: 1204,
  pluginCount: 5,
  crashFreeRate: 0.995,
  crashFreeTrend: 0.004,
  lastPublisher: { kind: "user", id: "u-ana", display: "Ana" },
  limits: [
    { key: "app.plugins", used: 5, limit: 200 },
    { key: "release.appSize", used: 52_428_800, limit: 104_857_600 },
    { key: "app.functions", used: 3, limit: 100 },
    { key: "app.locales", used: 4, limit: 50 },
  ],
};
const chat: AppsGridApp = {
  id: "a-chat",
  key: "chat",
  name: "Chat",
  tags: ["social"],
  lastPublishedAt: 1_800_000_000,
  releases: [{ environmentKey: "production", liveSequence: 9 }],
  activeDevices: 10,
  pluginCount: 170,
  crashFreeRate: 0.9,
  crashFreeTrend: -0.02,
  limits: [{ key: "app.plugins", used: 170, limit: 200 }],
};
const shop: AppsGridApp = { id: "a-shop", key: "shop", name: "Shop", owningTeamId: "t1", tags: ["retail"] };
const old: AppsGridApp = { id: "a-old", key: "old", name: "Old App", archived: true };

const manager = ["app.read", "app.manage", "plugin.read"];
const setup = (
  over: Partial<AppsGridFixture> = {},
  state: Upstream = {},
): { grid: AppsGridFixture; state: Upstream } => {
  const grid: AppsGridFixture = { apps: [{ ...bank }, { ...chat }, { ...shop }, { ...old }], calls: [], ...over };
  return { grid, state: { permissions: manager, admin: { teams }, appsGrid: grid, updates: [], ...state } };
};
const open = (state: Upstream, path = home) => renderApp(path, state);
const card = async (name: string): Promise<HTMLElement> => {
  const li = (await screen.findByRole("link", { name: `Open ${name}` }, wait)).closest("li");
  if (li === null) {
    throw new Error(`no card for ${name}`);
  }
  return li;
};
const names = () => screen.getAllByRole("link", { name: /^Open / }).map((a) => a.getAttribute("aria-label"));
const pick = async (combobox: string, option: string) => {
  fireEvent.click(await screen.findByRole("combobox", { name: combobox }));
  const item = await screen.findByRole("option", { name: option });
  // Base UI ignores a pick made at once after the popup opens: wait, then press and release as a pointer does.
  await new Promise((r) => setTimeout(r, 400));
  for (const type of ["pointerDown", "mouseDown", "pointerUp", "mouseUp", "click"] as const) {
    fireEvent[type](item);
  }
};
const menu = async (name: string, entry: string): Promise<HTMLElement> => {
  const trigger = await screen.findByRole("button", { name: `Actions for ${name}` }, wait);
  fireEvent.pointerDown(trigger, { button: 0 });
  fireEvent.click(trigger);
  return screen.findByRole("menuitem", { name: entry });
};

describe("the apps as a responsive grid with a list alternative [STU-020]", () => {
  test("active apps are cards in a grid that reflows from one column to many, in name order", async () => {
    const { state } = setup();
    open(state);
    await card("Bank");
    expect(names()).toEqual(["Open Bank", "Open Chat", "Open Shop"]);
    const grid = screen.getByRole("link", { name: "Open Bank" }).closest("ul");
    expect(grid?.className).toContain("grid-cols-1");
    expect(grid?.className).toContain("sm:grid-cols-2");
    expect(grid?.className).toContain("xl:grid-cols-3");
    expect(screen.getByText("3 apps")).toBeTruthy();
  });

  test("the list view shows the same apps as rows and is part of the URL", async () => {
    const { state } = setup();
    const r = open(state);
    await card("Bank");
    fireEvent.click(screen.getByRole("button", { name: "List" }));
    await waitFor(() => expect(r.router.state.location.search).toMatchObject({ view: "list" }));
    expect(screen.getByRole("button", { name: "List" }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("button", { name: "Grid" }).getAttribute("aria-pressed")).toBe("false");
    const row = (await card("Bank")).closest("ul");
    expect(row?.className).not.toContain("grid-cols-1");
    expect(names()).toEqual(["Open Bank", "Open Chat", "Open Shop"]);
  });

  test("the list view is opened by a link with the view in it", async () => {
    const { state } = setup();
    open(state, `${home}?view=list`);
    await card("Bank");
    expect(screen.getByRole("button", { name: "List" }).getAttribute("aria-pressed")).toBe("true");
  });

  test("the summaries are read in batches of app.summariesPerCall, never one call per card", async () => {
    const { grid, state } = setup({ perCall: 2 });
    open(state);
    await card("Shop");
    const size = limitDefault("app.summariesPerCall");
    expect(grid.calls.filter((c) => c.startsWith("summaries"))).toEqual([
      `summaries:${size}:`,
      `summaries:${size}:chat`,
    ]);
  });

  test("an organisation without apps shows the empty state, and without app.manage says whom to ask", async () => {
    const { state } = setup({ apps: [] }, { permissions: ["app.read"] });
    open(state);
    await screen.findByText("This organisation has no apps yet.", {}, wait);
    expect(screen.getByText("Ask an administrator to create one.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Create app" }) === null).toBe(true);
  });
});

describe("the card's visual [STU-021]", () => {
  test("a screenshot of the plugin canvas comes first, then the uploaded image, then the monogram", async () => {
    const assets = {
      "a-bank": [
        { id: "ic-1", name: "icon.png", mediaType: "image/png", url: "/assets/icon-bank.png" },
        { id: "th-1", name: "thumbnails/screen-home.png", mediaType: "image/png", url: "/assets/shot-bank.png" },
      ],
      "a-chat": [{ id: "ic-2", name: "icon.png", mediaType: "image/png", url: "/assets/icon-chat.png" }],
    };
    const { state } = setup({
      assets,
      apps: [{ ...bank, iconAssetId: "ic-1" }, { ...chat, iconAssetId: "ic-2" }, { ...shop }],
    });
    open(state);
    const shot = await screen.findByAltText("Screenshot of Bank", {}, wait);
    expect(shot.getAttribute("src")).toBe("/assets/shot-bank.png");
    const image = await screen.findByAltText("Image of Chat", {}, wait);
    expect(image.getAttribute("src")).toBe("/assets/icon-chat.png");
    const bankCard = await card("Bank");
    expect(bankCard.querySelector("[data-visual]")?.getAttribute("data-visual")).toBe("screenshot");
    expect((await card("Chat")).querySelector("[data-visual]")?.getAttribute("data-visual")).toBe("image");
    const monogram = (await card("Shop")).querySelector("[data-visual]");
    expect(monogram?.getAttribute("data-visual")).toBe("monogram");
    expect(monogram?.textContent).toBe("S");
  });
});

describe("what a card shows [STU-022]", () => {
  test("name, description, team, live release per environment, last publish, devices, plugins and health", async () => {
    const { state } = setup();
    open(state);
    const c = within(await card("Bank"));
    expect(c.getByText("Bank")).toBeTruthy();
    expect(c.getByText("Mobile banking for retail customers.")).toBeTruthy();
    expect(c.getByText("Team Payments")).toBeTruthy();
    const releases = within(c.getByRole("list", { name: "Live releases" }));
    expect(releases.getByText("production release 3")).toBeTruthy();
    expect(releases.getByText("staging not live")).toBeTruthy();
    expect(c.getByText(/^Last publish .*2023/)).toBeTruthy();
    expect(c.getByText("Active devices").nextElementSibling?.textContent).toBe("1,204");
    expect(c.getByText("Plugins", { selector: "dt" }).nextElementSibling?.textContent).toBe("5");
    expect(c.getByText("Healthy")).toBeTruthy();
    expect(c.getByText("99.5% crash-free")).toBeTruthy();
    expect(c.getByText("Errors falling, 0.4 points")).toBeTruthy();
  });

  test("a failing app is unhealthy with a rising error trend, and an app nobody uses has no rate", async () => {
    const { state } = setup();
    open(state);
    const failing = within(await card("Chat"));
    expect(failing.getByText("Unhealthy")).toBeTruthy();
    expect(failing.getByText("Errors rising, 2 points")).toBeTruthy();
    expect(failing.getByText("No team")).toBeTruthy();
    const unused = within(await card("Shop"));
    expect(unused.getByText("No devices")).toBeTruthy();
    expect(unused.getByText("Never published")).toBeTruthy();
    expect(unused.getByText("Nothing published yet")).toBeTruthy();
  });

  test("the name opens the app's canvas", async () => {
    const { state } = setup();
    open(state);
    const link = await screen.findByRole("link", { name: "Open Bank" }, wait);
    expect(link.getAttribute("href")).toBe(`/o/${orgA}/apps/a-bank`);
  });
});

describe("search, filters, sort, pins and actions [STU-023]", () => {
  test("the search narrows the apps and is part of the URL", async () => {
    const { state } = setup();
    const r = open(state);
    await card("Bank");
    fireEvent.change(screen.getByLabelText("Search apps"), { target: { value: "retail" } });
    await waitFor(() => expect(names()).toEqual(["Open Bank", "Open Shop"]));
    expect(r.router.state.location.search).toMatchObject({ q: "retail" });
    expect(screen.getByText("2 apps")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Search apps"), { target: { value: "nothing like it" } });
    await screen.findByText("No app matches these filters.");
    fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
    await waitFor(() => expect(names().length).toBe(3));
  });

  test("the team filter keeps the apps a team owns", async () => {
    const { state } = setup();
    open(state);
    await card("Bank");
    await pick("Team", "Payments");
    await waitFor(() => expect(names()).toEqual(["Open Bank"]));
  });

  test("the tag filter keeps the apps that carry the tag", async () => {
    const { state } = setup();
    open(state);
    await card("Bank");
    await pick("Tag", "social");
    await waitFor(() => expect(names()).toEqual(["Open Chat"]));
  });

  test("the status filter shows archived apps with a badge, which can be unarchived", async () => {
    const { grid, state } = setup();
    open(state);
    await card("Bank");
    expect(screen.queryByRole("link", { name: "Open Old App" }) === null).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Archived" }));
    const archived = within(await card("Old App"));
    expect(archived.getAllByText("Archived").length).toBeGreaterThan(0);
    expect(names()).toEqual(["Open Old App"]);
    fireEvent.click(await menu("Old App", "Unarchive"));
    await waitFor(() => expect(grid.calls).toContain("archive:a-old:off"));
    fireEvent.click(screen.getByRole("button", { name: "Active" }));
    await card("Old App");
  });

  test("my apps keeps the apps the caller holds a grant on", async () => {
    const { grid, state } = setup({ mine: ["a-chat"] });
    open(state);
    await card("Bank");
    fireEvent.click(screen.getByRole("button", { name: "My apps" }));
    await waitFor(() => expect(names()).toEqual(["Open Chat"]));
    expect(grid.calls).toContain("listApps:all:true");
  });

  test("sorting by health puts the apps that need attention first, by last activity the newest publish", async () => {
    const { state } = setup();
    open(state);
    await card("Bank");
    await pick("Sort by", "Health");
    await waitFor(() => expect(names()[0]).toBe("Open Chat"));
    cleanup();
    open(setup().state);
    await card("Bank");
    await pick("Sort by", "Last activity");
    await waitFor(() => expect(names()).toEqual(["Open Chat", "Open Bank", "Open Shop"]));
  });

  test("pinning saves the favourite apps in the preferences and moves the app to the top", async () => {
    const { state } = setup();
    open(state);
    await card("Shop");
    fireEvent.click(screen.getByRole("button", { name: "Pin Shop" }));
    await waitFor(() => expect(names()[0]).toBe("Open Shop"));
    expect(state.updates?.at(-1)).toMatchObject({ paths: ["favourite_app_ids"], favouriteAppIds: ["a-shop"] });
    expect(screen.getByRole("button", { name: "Unpin Shop" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "Unpin Shop" }));
    await waitFor(() => expect(names()[0]).toBe("Open Bank"));
    expect(state.updates?.at(-1)).toMatchObject({ favouriteAppIds: [] });
  });

  test("an app pinned earlier is first", async () => {
    const { state } = setup({}, { stored: { theme: "", language: "", density: "", favouriteAppIds: ["a-chat"] } });
    open(state);
    await card("Bank");
    await waitFor(() => expect(names()[0]).toBe("Open Chat"));
  });

  test("a manager archives an app from its menu", async () => {
    const { grid, state } = setup();
    open(state);
    await card("Bank");
    fireEvent.click(await menu("Bank", "Archive"));
    await waitFor(() => expect(grid.calls).toContain("archive:a-bank:on"));
    await waitFor(() => expect(screen.queryByRole("link", { name: "Open Bank" }) === null).toBe(true));
  });

  test("duplicating asks for a name and key, copies the app and opens the copy", async () => {
    const { grid, state } = setup();
    const r = open(state);
    await card("Bank");
    fireEvent.click(await menu("Bank", "Duplicate"));
    const dialog = await screen.findByRole("dialog", { name: "Duplicate Bank" });
    fireEvent.change(within(dialog).getByLabelText("Name"), { target: { value: "Bank 2" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Duplicate app" }));
    await waitFor(() => expect(grid.calls).toContain("duplicate:a-bank:bank-2:Bank 2"));
    await waitFor(() => expect(r.router.state.location.pathname).toBe(`/o/${orgA}/apps/app-bank-2`));
  });

  test("exporting downloads the app as a .plux archive", async () => {
    const { grid, state } = setup();
    const blobs: Blob[] = [];
    URL.createObjectURL = (b: Blob | MediaSource) => {
      blobs.push(b as Blob);
      return "blob:plux";
    };
    URL.revokeObjectURL = () => undefined;
    open(state);
    await card("Bank");
    fireEvent.click(await menu("Bank", "Export .plux archive"));
    await waitFor(() => expect(grid.calls).toContain("export:a-bank"));
    await waitFor(() => expect(blobs.length).toBe(1));
  });

  test("settings opens the app's settings page", async () => {
    const { state } = setup();
    const r = open(state);
    await card("Bank");
    fireEvent.click(await menu("Bank", "Settings"));
    await waitFor(() => expect(r.router.state.location.pathname).toBe(`/o/${orgA}/apps/a-bank/settings`));
  });

  test("actions the person's permissions do not allow are disabled", async () => {
    const { state } = setup({}, { permissions: ["app.read"] });
    open(state);
    await card("Bank");
    const archive = await menu("Bank", "Archive");
    expect(archive.getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("menuitem", { name: "Duplicate" }).getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("menuitem", { name: "Export .plux archive" }).getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("menuitem", { name: "Open" }).getAttribute("aria-disabled")).not.toBe("true");
    expect(screen.getByRole("menuitem", { name: "Settings" }).getAttribute("aria-disabled")).not.toBe("true");
  });
});

describe("the publisher of the last release on the card [STU-022]", () => {
  test("the card names who created the newest release, and says nothing when there is none", async () => {
    const { state } = setup();
    open(state);
    expect(within(await card("Bank")).getByText(/^Last publish .*2023.* by Ana$/)).toBeTruthy();
    expect(
      within(await card("Chat"))
        .getByText(/^Last publish /)
        .textContent?.includes(" by "),
    ).toBe(false);
    expect(within(await card("Shop")).getByText("Never published")).toBeTruthy();
  });
});

describe("limits usage on the card [STU-025]", () => {
  test("the card meters cover release size, functions and locales beside the plugins", async () => {
    const { state } = setup();
    open(state);
    const c = within(await card("Bank"));
    expect(c.getByRole("meter", { name: "Release size" }).getAttribute("aria-valuetext")).toBe("50 MiB of 100 MiB");
    expect(c.getByRole("meter", { name: "Functions" }).getAttribute("aria-valuetext")).toBe("3 of 100");
    expect(c.getByRole("meter", { name: "Locales" }).getAttribute("aria-valuetext")).toBe("4 of 50");
  });

  test("each card shows its usage against the limit in force, with a badge from 80 percent", async () => {
    const { state } = setup();
    open(state);
    const calm = within(await card("Bank"));
    const meter = calm.getByRole("meter", { name: "Plugins" });
    expect(meter.getAttribute("aria-valuenow")).toBe("5");
    expect(meter.getAttribute("aria-valuemax")).toBe("200");
    expect(meter.getAttribute("aria-valuetext")).toBe("5 of 200");
    expect(calm.queryByText("Near limit") === null).toBe(true);
    const busy = within(await card("Chat"));
    expect(busy.getByRole("meter", { name: "Plugins" }).getAttribute("aria-valuetext")).toBe("170 of 200");
    expect(busy.getByText("Near limit")).toBeTruthy();
  });
});
