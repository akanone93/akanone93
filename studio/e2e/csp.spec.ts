// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "@playwright/test";

/** The BFF serves the built app under its strict CSP (rule book §11.2): loading must not violate it. */
for (const path of ["/", "/signin", "/unsupported"]) {
  test(`${path} loads without a CSP violation [SEC-101]`, async ({ page }) => {
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

/**
 * Trusted Types are enforced, not only declared (P11 X5, SEC-109): a string reaching an HTML or script-URL sink is
 * refused, and no policy other than `plux-worker` can be created, not even `default`.
 */
test("Trusted Types refuse strings at sinks and unknown policies [SEC-101]", async ({ page }) => {
  await page.goto("/signin");
  await page.waitForLoadState("networkidle");
  const refused = await page.evaluate(() => {
    const throws = (f: () => unknown): boolean => {
      try {
        f();
        return false;
      } catch (e) {
        return e instanceof TypeError;
      }
    };
    const tt = (window as unknown as { trustedTypes?: { createPolicy(n: string, r: object): unknown } }).trustedTypes;
    return {
      trustedTypes: tt !== undefined,
      innerHTML: throws(() => {
        // biome-ignore lint/plugin: the test proves the browser refuses a string at this sink
        document.createElement("div").innerHTML = "<img src=x>";
      }),
      scriptSrc: throws(() => {
        document.createElement("script").src = "/assets/x.js";
      }),
      worker: throws(() => new Worker("/assets/x.js")),
      foreignPolicy: throws(() => tt?.createPolicy("attacker", { createHTML: (s: string) => s })),
      defaultPolicy: throws(() => tt?.createPolicy("default", { createHTML: (s: string) => s })),
    };
  });
  expect(refused).toEqual({
    trustedTypes: true,
    innerHTML: true,
    scriptSrc: true,
    worker: true,
    foreignPolicy: true,
    defaultPolicy: true,
  });
});
