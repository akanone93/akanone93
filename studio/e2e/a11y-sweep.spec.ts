// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
import { orgA } from "../apps/web/src/test-upstream.ts";
import { mockUpstream } from "./upstream.ts";

const axeTags = ["wcag2a", "wcag2aa", "wcag21aa"];

const permissions = [
  "app.read",
  "audit.read",
  "compliance.report",
  "limits.manage",
  "members.manage",
  "organization.manage",
  "release.publish",
  "release.promote",
  "rollout.manage",
  "function.deploy",
  "restrictions.manage",
  "retention.manage",
  "roles.manage",
  "sessions.manage",
  "sso.manage",
  "teams.manage",
  "tokens.manage",
];

const org = `/o/${orgA}`;
const app = `${org}/apps/app1`;

/** One entry per area route of router.tsx; detail routes use ids the fixtures do not know, which render the area's empty or not-found state. */
const routes: readonly { name: string; path: string; ready?: (p: Page) => Promise<void> }[] = [
  { name: "shell", path: org },
  { name: "notifications", path: `${org}/notifications` },
  { name: "profile", path: "/me" },
  { name: "shortcuts", path: "/me/shortcuts" },
  { name: "app canvas", path: app },
  {
    name: "plugin canvas",
    path: `${app}/plugins/pl`,
    // The plugin canvas boots its worker, CanvasKit and fonts first.
    ready: (p) => expect(p.getByRole("application")).toBeVisible({ timeout: 30_000 }),
  },
  { name: "releases", path: `${app}/releases` },
  { name: "kill switch", path: `${app}/releases/kill-switch` },
  { name: "release", path: `${app}/releases/1` },
  { name: "functions", path: `${app}/functions` },
  { name: "function", path: `${app}/functions/f1` },
  { name: "function edit", path: `${app}/functions/f1/edit` },
  { name: "data sources", path: `${app}/data` },
  { name: "data source", path: `${app}/data/sources/s1` },
  { name: "data collections", path: `${app}/data/collections` },
  { name: "data collection", path: `${app}/data/collections/c1` },
  { name: "design tokens", path: `${app}/design` },
  { name: "design themes", path: `${app}/design/themes` },
  { name: "design typography", path: `${app}/design/typography` },
  { name: "design icons", path: `${app}/design/icons` },
  { name: "design library", path: `${app}/design/library` },
  { name: "localisation", path: `${app}/localisation` },
  { name: "localisation heatmap", path: `${app}/localisation/heatmap` },
  { name: "analytics", path: `${app}/analytics` },
  { name: "analytics usage", path: `${app}/analytics/usage` },
  { name: "analytics funnels", path: `${app}/analytics/funnels` },
  { name: "analytics errors", path: `${app}/analytics/errors` },
  { name: "analytics replays", path: `${app}/analytics/replays` },
  { name: "tests", path: `${app}/tests` },
  { name: "test run", path: `${app}/tests/run1` },
  { name: "experiments", path: `${app}/experiments` },
  { name: "new experiment", path: `${app}/experiments/new` },
  { name: "flags", path: `${app}/experiments/flags` },
  { name: "experiment", path: `${app}/experiments/e1` },
  { name: "approvals", path: `${org}/approvals` },
  { name: "approvals history", path: `${org}/approvals/history` },
  { name: "break glass", path: `${org}/approvals/break-glass` },
  { name: "approval policies", path: `${org}/approvals/policies` },
  { name: "approval policy", path: `${org}/approvals/policies/p1` },
  { name: "approval request", path: `${org}/approvals/req1` },
  { name: "publish", path: `${app}/publish` },
  { name: "publish request", path: `${app}/publish/req1` },
  { name: "devices", path: `${app}/devices` },
  { name: "device", path: `${app}/devices/d1` },
  { name: "security profile", path: `${app}/security` },
  { name: "security keys", path: `${app}/security/keys` },
  { name: "security pins", path: `${app}/security/pins` },
  { name: "security capabilities", path: `${app}/security/capabilities` },
  ...[
    "",
    "/members",
    "/members/u1",
    "/teams",
    "/teams/t1",
    "/roles",
    "/roles/r1",
    "/restrictions",
    "/access",
    "/sso",
    "/tokens",
    "/sessions",
    "/audit",
    "/compliance",
    "/retention",
    "/license",
    "/limits",
  ].map((suffix) => ({ name: `admin${suffix}`, path: `${org}/admin${suffix}` })),
  ...["", "/organisations", "/storage", "/jobs", "/backups", "/issuers", "/audit"].map((suffix) => ({
    name: `installation${suffix}`,
    path: `/installation${suffix}`,
  })),
  { name: "library", path: `${org}/library` },
  { name: "library component", path: `${org}/library/components/c1` },
  { name: "library template", path: `${org}/library/templates/t1` },
];

const pageOf = (id: string, key: string) => ({
  kind: "page",
  path: `plugins/pl/pages/${key}.page.json`,
  json: {
    id,
    key,
    route: key,
    title: key,
    kind: "page",
    pageKind: "screen",
    root: { id: `${id}-root`, type: "ElevatedButton", events: { onPressed: { steps: [] } } },
  },
});
const pluginDoc = {
  kind: "plugin",
  path: "plugins/pl/plugin.json",
  json: { kind: "plugin", id: "pl", key: "pl", name: "pl", pages: ["home"], entryPage: "home" },
};

/** Visits one area route and asserts zero axe violations; a failure lists the rule ids with the nodes. */
async function check(p: Page, route: (typeof routes)[number]): Promise<void> {
  await p.setViewportSize({ width: 1280, height: 800 });
  await mockUpstream(p, {
    permissions,
    installationAdmin: true,
    summaries: [{ id: "pl", key: "pl", name: "Loans" }],
    documents: [pluginDoc, pageOf("home", "home")],
  });
  await p.goto(route.path);
  await expect(p.locator("main")).toBeVisible({ timeout: 30_000 });
  await route.ready?.(p);
  await expect(p.locator("html")).toHaveAttribute("data-theme", /^(light|dark)$/);
  // Long-lived requests (fonts, event streams) keep a page from ever being network-idle: wait for the loading states to end instead.
  await expect(p.locator('[aria-busy="true"], [role="progressbar"]')).toHaveCount(0);
  const { violations } = await new AxeBuilder({ page: p }).withTags(axeTags).analyze();
  expect(
    violations.map(
      (v) => `${v.id}: ${v.nodes.map((n) => `${n.target.join(" ")} ${n.any[0]?.message ?? ""}`).join(" | ")}`,
    ),
  ).toEqual([]);
}

test.describe.configure({ timeout: 60_000 });

test.describe("axe in light [STU-011]", () => {
  test.use({ colorScheme: "light" });
  for (const route of routes) {
    test(route.name, async ({ page }) => {
      await check(page, route);
      await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
    });
  }
});

test.describe("axe in dark [STU-011 STU-014]", () => {
  test.use({ colorScheme: "dark" });
  for (const route of routes) {
    test(route.name, async ({ page }) => {
      await check(page, route);
      await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
    });
  }
});

test.describe("axe in RTL, Arabic [STU-011 STU-012]", () => {
  test.use({ locale: "ar" });
  for (const route of routes) {
    test(route.name, async ({ page }) => {
      await check(page, route);
      await expect(page.locator("html")).toHaveAttribute("dir", "rtl");
      await expect(page.locator("html")).toHaveAttribute("lang", "ar");
    });
  }
});
