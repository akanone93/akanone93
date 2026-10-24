// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

// The catalogue page exists only under the Vite dev server (see playwright.config.ts, project "dev").
const variants = [
  { name: "light", query: "theme=light" },
  { name: "dark", query: "theme=dark" },
  { name: "contrast", query: "theme=contrast" },
  { name: "ar-XB", query: "theme=light&dir=rtl&lang=ar-XB" },
] as const;

for (const variant of variants) {
  test(`the catalogue has no WCAG 2.1 AA violations in ${variant.name} [STU-011]`, async ({ page }) => {
    await page.goto(`/__catalogue?${variant.query}`);
    await expect(page.getByRole("main")).toBeAttached();
    const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze();
    expect(results.violations).toEqual([]);
  });
}

// These cases open an overlay on mount, so each renders alone.
for (const only of ["dialog", "menu", "popover"]) {
  test(`the ${only} cases have no WCAG 2.1 AA violations [STU-011]`, async ({ page }) => {
    await page.goto(`/__catalogue?only=${only}`);
    await expect(page.locator(`[data-case^="${only}/"]`).first()).toBeAttached();
    await page.waitForTimeout(500);
    const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze();
    expect(results.violations).toEqual([]);
  });
}

test("the Tooltip opens on hover and on keyboard focus [STU-011]", async ({ page }) => {
  await page.goto("/__catalogue");
  const trigger = page.locator('[data-case="tooltip/icon button"]').getByRole("button", { name: "Add a page" });
  await trigger.hover();
  await expect(page.getByText("Add a page", { exact: true })).toBeVisible();
  await page.mouse.move(0, 0);
  await expect(page.getByText("Add a page", { exact: true })).toBeHidden();
  await trigger.focus();
  await page.keyboard.press("Shift+Tab");
  await page.keyboard.press("Tab");
  await expect(page.getByText("Add a page", { exact: true })).toBeVisible();
});
