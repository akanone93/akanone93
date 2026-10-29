// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { orgA } from "../apps/web/src/test-upstream.ts";
import { mockUpstream } from "./upstream.ts";

const home = {
  kind: "page",
  path: "plugins/pl/pages/home.page.json",
  json: {
    id: "home",
    key: "home",
    route: "home",
    title: "home",
    kind: "page",
    pageKind: "screen",
    root: { id: "home-root", type: "Text", props: { data: "Hello" } },
  },
};
const plugin = {
  kind: "plugin",
  path: "plugins/pl/plugin.json",
  json: { kind: "plugin", id: "pl", key: "pl", name: "pl", pages: ["home"], entryPage: "home" },
};

test("keyboard only: open a plugin, select a widget, edit a property, undo, open publish [STU-011]", async ({
  page: p,
}) => {
  await p.setViewportSize({ width: 1280, height: 800 });
  await mockUpstream(p, {
    permissions: ["app.read", "release.publish"],
    summaries: [{ id: "pl", key: "pl", name: "Loans" }],
    documents: [plugin, home],
  });

  // Open a plugin: the app canvas lists its boxes as buttons.
  await p.goto(`/o/${orgA}/apps/app1`);
  const box = p.getByRole("list", { name: "Plugins" }).getByRole("button", { name: "Loans, plugin pl" });
  await expect(box).toBeAttached();
  await box.focus();
  await p.keyboard.press("Enter");
  await expect(p).toHaveURL(new RegExp(`/o/${orgA}/apps/app1/plugins/pl`));
  // The plugin canvas boots its worker, CanvasKit and fonts first; on a loaded machine that exceeds the 5 s default.
  await expect(p.getByRole("button", { name: "Undo" })).toBeVisible({ timeout: 30_000 });

  // Select a widget: the Layers tab is a tab list (arrow keys) and its tree items select with Enter.
  const layers = p.getByRole("tab", { name: "Layers" });
  await layers.focus();
  await p.keyboard.press("Enter");
  const item = p.locator('[role="treeitem"][data-item-id="home-root"]');
  await item.focus();
  await p.keyboard.press("Enter");

  // Edit a property: the generated field commits when it loses focus, which Tab does.
  const data = p.getByRole("textbox", { name: "data" });
  await expect(data).toHaveValue("Hello");
  await data.focus();
  await p.keyboard.press("ControlOrMeta+A");
  await p.keyboard.type("Hi");
  await p.keyboard.press("Tab");
  await expect(data).toHaveValue("Hi");

  // Undo from the action bar's button.
  const undo = p.getByRole("button", { name: "Undo" });
  await expect(undo).toBeEnabled();
  await undo.focus();
  await p.keyboard.press("Enter");
  await expect(data).toHaveValue("Hello");
  expect((await new AxeBuilder({ page: p }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze()).violations).toEqual(
    [],
  );

  // Open publish.
  const publish = p.getByRole("button", { name: "Publish" });
  await publish.focus();
  await p.keyboard.press("Enter");
  await expect(p).toHaveURL(new RegExp(`/o/${orgA}/apps/app1/publish`));
});
