// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import AxeBuilder from "@axe-core/playwright";
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
      page("home", "home", [{ id: "go", action: "navigate", input: { route: "detail" } }]),
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

const axeTags = ["wcag2a", "wcag2aa", "wcag21aa"];

test("S3 flow: app canvas, plugin canvas, lane entry and blocked delete [STU-030 STU-046 STU-047 STU-070]", async ({
  page: p,
}) => {
  await mockUpstream(p);

  // C1: the app canvas draws one box per plugin; the accessible list names it.
  await p.goto(`/o/${orgA}/apps/app1`);
  const boxes = p.getByRole("list", { name: "Plugins" });
  await expect(boxes.getByRole("button", { name: "Loans, plugin pl" })).toBeAttached();
  expect((await new AxeBuilder({ page: p }).withTags(axeTags).analyze()).violations).toEqual([]);

  // C2: a double click is not needed; the box opens by keyboard (Enter) as the list says.
  await boxes.getByRole("button", { name: "Loans, plugin pl" }).focus();
  await p.keyboard.press("Enter");
  await expect(p).toHaveURL(new RegExp(`/o/${orgA}/apps/app1/plugins/pl`));
  await expect(p.getByRole("button", { name: "Undo" })).toBeVisible();
  await expect(p.getByRole("list", { name: "Screens" })).toBeAttached();
  expect((await new AxeBuilder({ page: p }).withTags(axeTags).analyze()).violations).toEqual([]);

  // The link home -> detail is listed as text for the selected widget: the arrow's accessible entry.
  await p.getByRole("tab", { name: "Layers" }).click();
  await p.locator('[role="treeitem"][data-item-id="home-root"]').click();
  await p.getByRole("tab", { name: "Actions" }).click();
  await expect(p.getByText("onPressed goes to detail")).toBeVisible();
  expect((await new AxeBuilder({ page: p }).withTags(axeTags).analyze()).violations).toEqual([]);

  // The target screen is now used by that link: deleting it is blocked and says why.
  await p.getByRole("tab", { name: "Layers" }).click();
  await p.locator('[role="treeitem"][data-item-id="detail-root"]').click();
  // The action bar sits under the left panel at this width: the menu opens from the keyboard.
  await p.getByRole("button", { name: "Screen menu" }).focus();
  await p.keyboard.press("Enter");
  await p.getByRole("menuitem", { name: "Delete screen" }).focus();
  await p.keyboard.press("Enter");
  await expect(p.getByText("This cannot be deleted yet")).toBeVisible();
});
