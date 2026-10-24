// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";

/** With no upstream behind the preview server, the BFF answers 401: the app shows the sign-in page. */
async function signedOutUpstream(page: Page) {
  await page.route("**/api/plux.v1.*/*", (route) =>
    route.fulfill({
      status: 401,
      contentType: "application/json",
      body: JSON.stringify({ code: "unauthenticated", message: "no session" }),
    }),
  );
}

test("the app loads and has a main landmark [STU-011]", async ({ page }) => {
  await signedOutUpstream(page);
  await page.goto("/");
  await expect(page).toHaveTitle("Plux Studio");
  await expect(page.getByRole("main")).toBeAttached();
});

test("the app has no WCAG 2.1 AA accessibility violations [STU-011]", async ({ page }) => {
  await signedOutUpstream(page);
  await page.goto("/");
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze();
  expect(results.violations).toEqual([]);
});
