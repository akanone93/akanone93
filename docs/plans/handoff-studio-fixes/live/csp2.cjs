// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0
const { open } = require('./common.cjs');
(async () => {
  const { browser, page, log, env } = await open();
  await page.addInitScript(() => {
    window.__csp = [];
    document.addEventListener('securitypolicyviolation', (e) => {
      window.__csp.push({ d: e.violatedDirective, src: (e.sourceFile || '').replace(/.*\/assets\//, ''), line: e.lineNumber, col: e.columnNumber, sample: e.sample });
    });
  });
  const o = env.PLUX_DEV_ORGANIZATION, a = env.PLUX_DEV_APP;
  const routes = [`/o/${o}`, `/o/${o}/apps/${a}`, `/o/${o}/apps/${a}/settings`, `/o/${o}/apps/${a}/publish`, `/o/${o}/library`, `/me`, `/o/${o}/notifications`, `/o/${o}/apps/${a}/plugins/01a1bc94-c7c4-745a-8d53-ae2dc3b03746`];
  const all = [];
  for (const r of routes) {
    await page.goto('http://localhost:8081' + r);
    await page.waitForTimeout(r.includes('plugins') ? 9000 : 3500);
    if (r.includes('plugins')) {
      for (const name of ['JSON', 'Code', 'Accessibility', 'Performance', 'Docs']) {
        await page.locator('button,[role=tab]').filter({ hasText: new RegExp('^' + name + '$') }).first().click().catch(() => {});
        await page.waitForTimeout(2500);
      }
      await page.getByRole('button', { name: /Preview/ }).first().click().catch(() => {});
      await page.waitForTimeout(1500);
      await page.keyboard.press('Escape');
    }
    const v = await page.evaluate(() => window.__csp.splice(0));
    for (const x of v) all.push({ r, ...x });
  }
  const agg = {};
  for (const x of all) { const k = `${x.r.replace(/[0-9a-f-]{36}/g, ':id')} ${x.d} ${x.src}:${x.line}:${x.col}`; agg[k] = (agg[k] || 0) + 1; }
  console.log('total', all.length); for (const [k, n] of Object.entries(agg)) console.log(n + 'x ' + k);
  console.log('csp console msgs', log.console.filter(c => /Content Security|style-src/.test(c)).length);
  await browser.close();
})();
