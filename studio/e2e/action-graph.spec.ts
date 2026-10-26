// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type DescMethodUnary, fromBinary, toBinary } from "@bufbuild/protobuf";
import { expect, type Page, test } from "@playwright/test";
import {
  AppService,
  DocumentService,
  FederationService,
  GovernanceService,
  IdentityService,
  NotificationService,
  OrgService,
  PluginService,
  PreferencesService,
} from "@plux/api";
import { createUpstream, orgA } from "../apps/web/src/test-upstream.ts";

const services = [
  IdentityService,
  OrgService,
  PreferencesService,
  GovernanceService,
  NotificationService,
  AppService,
  PluginService,
  DocumentService,
  FederationService,
];

const page = (id: string, key: string, steps: object[] = []) => ({
  kind: "page",
  path: `plugins/pl/pages/${key}.page.json`,
  json: {
    id,
    key,
    route: key,
    title: key,
    kind: "page",
    pageKind: "screen",
    root: { id: `${id}-root`, type: "ElevatedButton", events: { onPressed: { steps } } },
  },
});
const plugin = {
  kind: "plugin",
  path: "plugins/pl/plugin.json",
  json: { kind: "plugin", id: "pl", key: "pl", name: "pl", pages: ["home", "detail"], entryPage: "home" },
};

/** The BFF's /api/* answered by the same in-memory upstream the unit tests use: real Connect messages, no live server. */
async function mockUpstream(p: Page) {
  const transport = createUpstream({
    summaries: [{ id: "pl", key: "pl", name: "Loans" }],
    documents: [
      plugin,
      page("home", "home", [
        { id: "check", action: "kvGet", input: { key: "last" }, onSuccess: "go" },
        { id: "go", action: "navigate", input: { route: "detail" } },
      ]),
      page("detail", "detail"),
    ],
  });
  await p.route("**/api/plux.v1.*/*", async (route) => {
    const [, , typeName, name] = new URL(route.request().url()).pathname.split("/");
    const desc = services.find((s) => s.typeName === typeName)?.methods.find((m) => m.name === name);
    if (desc === undefined) {
      return route.fulfill({
        status: 404,
        contentType: "application/json",
        body: JSON.stringify({ code: "unimplemented" }),
      });
    }
    try {
      const input = fromBinary(desc.input, route.request().postDataBuffer() ?? new Uint8Array());
      const res = await transport.unary(desc as DescMethodUnary, undefined, undefined, undefined, input as never);
      return route.fulfill({
        status: 200,
        contentType: "application/proto",
        body: Buffer.from(toBinary(desc.output, res.message as never)),
      });
    } catch (e) {
      const err = e as { code: number; rawMessage: string };
      return route.fulfill({
        status: 400,
        contentType: "application/json",
        body: JSON.stringify({ code: String(err.code), message: err.rawMessage }),
      });
    }
  });
}

/**
 * The built app behind the BFF and its strict CSP (rule book §11.2): the action graph editor opens on the
 * selected widget, React Flow draws its steps, and a step is added by keyboard, all without a CSP violation
 * (STU-063, STU-065).
 */
test("the action graph editor opens under the strict CSP and edits by keyboard [STU-063, STU-065]", async ({
  page: p,
}) => {
  await p.addInitScript(() => {
    const w = window as unknown as { __csp: string[] };
    w.__csp = [];
    document.addEventListener("securitypolicyviolation", (e) => {
      w.__csp.push(`${e.violatedDirective} ${e.blockedURI} ${e.sample}`);
    });
  });
  await mockUpstream(p);
  // The BFF serves the app's shell at "/" only; the router takes the plugin canvas's address from there.
  await p.goto("/");
  await expect(p.getByRole("banner")).toBeVisible();
  await p.evaluate((to) => {
    history.pushState({}, "", to);
    dispatchEvent(new PopStateEvent("popstate"));
  }, `/o/${orgA}/apps/app1/plugins/pl`);
  await p.getByRole("tab", { name: "Layers" }).click();
  await p.locator('[role="treeitem"][data-item-id="home-root"]').click();
  await p.getByRole("button", { name: "Action graph" }).click();

  const nodes = p.locator(".react-flow__node");
  await expect(nodes).toHaveCount(2, { timeout: 30_000 });
  await expect(p.locator('section[aria-label="go"]')).toBeVisible();
  await expect(p.locator(".react-flow__edge")).toHaveCount(1);

  // Adding a step is a button in the catalogue, not a drag; the new step continues from the last one.
  await p.getByRole("button", { name: "Add pop to the graph" }).focus();
  await p.keyboard.press("Enter");
  await expect(nodes).toHaveCount(3);
  await expect(p.locator(".react-flow__edge")).toHaveCount(2);
  expect(await p.evaluate(() => (window as unknown as { __csp: string[] }).__csp)).toEqual([]);
});
