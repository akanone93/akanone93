// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import type { RollupPoint } from "@plux/api";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { orgA, renderApp, type Upstream } from "../../test-harness.tsx";
import { dailyEvents, percentilesByDay, totalsBy } from "./analytics.model.ts";
import { linePoints } from "./charts.tsx";
import { rangeOf } from "./use-analytics.ts";

afterEach(cleanup);

const base = `/o/${orgA}/apps/app1/analytics`;
const wait = { timeout: 5000 };
const analytics = {
  rollups: {
    screen_view: [
      { day: "2026-10-20", events: 4, plugin: "shop", page: "home" },
      { day: "2026-10-20", events: 2, plugin: "shop", page: "cart" },
      { day: "2026-10-21", events: 6, plugin: "blog", page: "post" },
    ],
    render_perf: [{ day: "2026-10-20", events: 3, p50: 8, p90: 16, p99: 40 }],
  },
  overview: [{ appId: "app1", sessions: 12, screenViews: 12, errors: 1, firingAlerts: 2 }],
  funnels: [{ id: "f1", name: "Checkout", steps: ["screen_view", "action_run"], counts: [10, 4] }],
  errors: [
    { id: "g1", fingerprint: "fp1", code: "PLX-4101", pluginKey: "shop", route: "cart", nodePath: "root/0", events: 5 },
  ],
};
const open = (path: string, state: Upstream = {}, permissions = ["app.read", "alerts.manage"]) =>
  renderApp(`${base}${path}`, { permissions, analytics: structuredClone(analytics), ...state });

describe("STU-114 analytics model", () => {
  test("sums events per day and per dimension", () => {
    const points = analytics.rollups.screen_view.map((r) => ({
      bucket: { seconds: BigInt(Date.parse(`${r.day}T00:00:00Z`) / 1000), nanos: 0 },
      events: BigInt(r.events),
      group: { pluginKey: r.plugin },
      p50: 0,
      p90: 0,
      p99: 0,
    }));
    const rows = points as unknown as RollupPoint[];
    expect(dailyEvents(rows)).toEqual([
      { key: "2026-10-20", value: 6 },
      { key: "2026-10-21", value: 6 },
    ]);
    expect(totalsBy(rows, (p) => p.group?.pluginKey ?? "")).toEqual([
      { key: "blog", value: 6 },
      { key: "shop", value: 6 },
    ]);
    expect(percentilesByDay([]).days).toEqual([]);
  });

  test("draws the largest value at the top and the first at the left", () => {
    expect(linePoints([0, 10], 10)).toBe("8.0,152.0 472.0,8.0");
  });

  test("defaults to the last seven days and honours ?from and ?to", () => {
    const r = rangeOf({}, new Date("2026-10-28T12:00:00Z"));
    expect(r.from.toISOString()).toBe("2026-10-22T00:00:00.000Z");
    expect(r.to.toISOString()).toBe("2026-10-29T00:00:00.000Z");
    expect(rangeOf({ from: "2026-10-01", to: "2026-10-03" }).to.toISOString()).toBe("2026-10-04T00:00:00.000Z");
  });
});

describe("STU-114 analytics pages (A24, A25, A30)", () => {
  test("A24 shows the headline numbers and the daily chart", async () => {
    open("");
    await screen.findByRole("img", { name: "Screen views per day" }, wait);
    expect(document.body.textContent).toContain("Firing alerts2");
  });

  test("A25 drills down from plugins to pages to one page", async () => {
    open("/usage");
    await screen.findByRole("list", { name: "Screen views by plugin" }, wait);
    fireEvent.click(screen.getByRole("link", { name: "shop" }));
    await screen.findByRole("list", { name: "Screen views by page" }, wait);
    expect(screen.queryByRole("link", { name: "blog" })).toBeNull();
    fireEvent.click(screen.getByRole("link", { name: "home" }));
    await screen.findByRole("img", { name: "Screen views of the page per day" }, wait);
  });

  test("A30 shows the percentiles", async () => {
    open("/performance");
    await screen.findByRole("img", { name: "Render time percentiles per day" }, wait);
    expect(screen.getAllByText("40").length).toBeGreaterThan(0);
  });

  test("shows the empty state without events", async () => {
    open("/performance", { analytics: { rollups: {} } });
    await screen.findByText("No data in this range.", {}, wait);
  });
});

describe("ANL-020 funnels (A26, A27)", () => {
  test("A26 creates and deletes a funnel; a reader cannot", async () => {
    open("/funnels");
    await screen.findByRole("link", { name: "Checkout" }, wait);
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Signup" } });
    fireEvent.change(screen.getByLabelText("Steps, one event name per line"), {
      target: { value: "screen_view\naction_run" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create funnel" }));
    await screen.findByRole("link", { name: "Signup" }, wait);
    const row = screen.getByRole("link", { name: "Signup" }).closest("tr") as HTMLElement;
    fireEvent.click(within(row).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(screen.queryByRole("link", { name: "Signup" })).toBeNull());
  }, 30_000);

  test("A26 disables the form without alerts.manage", async () => {
    open("/funnels", {}, ["app.read"]);
    await screen.findByRole("link", { name: "Checkout" }, wait);
    expect(screen.getByRole("button", { name: "Create funnel" }).matches(":disabled, [aria-disabled=true]")).toBe(true);
  });

  test("A27 shows the conversion of each step", async () => {
    open("/funnels/f1");
    await screen.findByRole("list", { name: "Events per funnel step" }, wait);
    expect(screen.getByText("40%")).toBeTruthy();
  });
});

describe("ANL-040 errors (A28, A29)", () => {
  test("A28 lists the groups and A29 opens one with its node link", async () => {
    open("/errors");
    fireEvent.click(await screen.findByRole("link", { name: "PLX-4101" }, wait));
    await screen.findByText("root/0", {}, wait);
    const node = screen.getByRole("link", { name: "Open node" });
    expect(node.getAttribute("href")).toContain("/plugins/shop");
    expect(screen.getByText("ev1")).toBeTruthy();
  });

  test("A29 reports an unknown fingerprint", async () => {
    open("/errors/nope");
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  });
});
