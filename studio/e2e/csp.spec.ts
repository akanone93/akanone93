// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "@playwright/test";

/** The BFF serves the built app under its strict CSP (rule book §11.2): loading must not violate it. */
for (const path of ["/", "/signin", "/unsupported"]) {
  test(`${path} loads without a CSP violation [STU-011]`, async ({ page }) => {
    await page.addInitScript(() => {
      const w = window as unknown as { __csp: string[] };
      w.__csp = [];
      document.addEventListener("securitypolicyviolation", (e) => {
        w.__csp.push(`${e.violatedDirective} ${e.blockedURI} ${e.sample}`);
      });
    });
    await page.goto(path);
    await page.waitForLoadState("networkidle");
    expect(await page.evaluate(() => (window as unknown as { __csp: string[] }).__csp)).toEqual([]);
  });
}
