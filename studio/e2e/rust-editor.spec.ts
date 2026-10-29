// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create, type DescMessage, type MessageShape, toBinary } from "@bufbuild/protobuf";
import { expect, test } from "@playwright/test";
import {
  DocumentSchema,
  GetCurrentUserResponseSchema,
  GetPreferencesResponseSchema,
  ListDocumentsResponseSchema,
  ListOrganizationsResponseSchema,
  MemberSchema,
  OrganizationSchema,
  PreferencesSchema,
  UserSchema,
} from "@plux/api";

const org = "0192f000-0000-7000-8000-00000000000a";

function proto<S extends DescMessage>(schema: S, init: Parameters<typeof create<S>>[1]) {
  return {
    status: 200,
    contentType: "application/proto",
    body: Buffer.from(toBinary(schema, create(schema, init) as MessageShape<S>)),
  };
}

const plugin = { id: "pl", kind: "plugin", entryPage: "home", pages: ["home"] };

/**
 * The built app behind the BFF and its strict CSP (rule book §11.2): the Rust editor opens, Monaco's code, its Rust
 * tokenizer chunk and its workers load, and the starter template renders tokenized, all without a single CSP or
 * Trusted Types violation (FN-033, STU-063). The language server is not started: the editor works without it.
 */
test("the Rust editor opens under the strict CSP and Trusted Types without a violation [FN-033, STU-063]", async ({
  page,
}) => {
  await page.addInitScript(() => {
    const w = window as unknown as { __csp: string[] };
    w.__csp = [];
    document.addEventListener("securitypolicyviolation", (e) => {
      w.__csp.push(`${e.violatedDirective} ${e.blockedURI} ${e.sample}`);
    });
  });
  const workers: string[] = [];
  page.on("worker", (w) => workers.push(w.url()));
  await page.route("**/api/plux.v1.*/*", async (route) => {
    const method = new URL(route.request().url()).pathname.split("/").pop();
    switch (method) {
      case "GetCurrentUser":
        return route.fulfill(
          proto(GetCurrentUserResponseSchema, {
            user: create(UserSchema, { id: "u1", displayName: "Ada", email: "ada@example.org" }),
            memberships: [create(MemberSchema, { organizationId: org, userId: "u1" })],
            csrfToken: "csrf",
          }),
        );
      case "ListOrganizations":
        return route.fulfill(
          proto(ListOrganizationsResponseSchema, {
            organizations: [create(OrganizationSchema, { id: org, key: "alpha", name: "Alpha Org" })],
          }),
        );
      case "GetPreferences":
        return route.fulfill(proto(GetPreferencesResponseSchema, { preferences: create(PreferencesSchema, {}) }));
      case "ListDocuments":
        return route.fulfill(
          proto(ListDocumentsResponseSchema, {
            documents: [
              create(DocumentSchema, {
                kind: "plugin",
                path: "plugin.json",
                revision: 1n,
                content: new TextEncoder().encode(JSON.stringify(plugin)),
              }),
            ],
          }),
        );
      default:
        return route.fulfill({
          status: 404,
          contentType: "application/json",
          body: JSON.stringify({ code: "not_found", message: "unmocked" }),
        });
    }
  });
  // The BFF serves the app's shell at "/" only; the router takes the plugin canvas's address from there.
  await page.goto("/");
  await expect(page.getByRole("banner")).toBeVisible();
  await page.evaluate((to) => {
    history.pushState({}, "", to);
    dispatchEvent(new PopStateEvent("popstate"));
  }, `/o/${org}/apps/app1/plugins/pl`);
  await page.getByRole("button", { name: "Rust editor" }).click();
  const editor = page.getByRole("textbox", { name: /Rust source of the function/ });
  await expect(editor).toBeAttached({ timeout: 30_000 });
  await expect(page.locator(".monaco-editor .view-lines")).toContainText("plux_fn");
  // The Rust language features come from the language server, so Monaco may start no worker of its own; every
  // worker the page does start is one of Studio's hashed same-origin assets.
  expect(workers.every((u) => /\/assets\/(monaco-editor|canvas|lock|validator)-worker-[\w-]+\.js$/.test(u))).toBe(true);
  // The Rust tokenizer loads on demand; once it has, the lines carry token classes beyond the default.
  await expect(page.locator('.monaco-editor .view-line span[class*="mtk"]').first()).toBeAttached();
  await expect(page.getByRole("button", { name: "Start the language server" })).toBeDisabled();
  await page.getByRole("tab", { name: "Dependencies" }).click();
  await expect(page.getByText("The function has no dependencies.")).toBeVisible();
  expect(await page.evaluate(() => (window as unknown as { __csp: string[] }).__csp)).toEqual([]);
});
