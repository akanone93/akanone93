// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0
const { open, summary } = require('./common.cjs');
const { execSync } = require('child_process');
async function overlayBox(p, kind = 'selected') {
  return p.evaluate((k) => {
    const el = document.querySelector(`.pc-outline[data-kind="${k}"]`);
    const c = document.querySelector('canvas[role=application]')?.getBoundingClientRect();
    if (!el || !c) return null;
    const v = (n) => Number.parseFloat(el.style.getPropertyValue(n));
    return { x: c.left + v('--x'), y: c.top + v('--y'), w: v('--w'), h: v('--h') };
  }, kind);
}
(async () => {
  const { browser, page, log, env } = await open();
  const plugin = '01a1bc94-c7c4-745a-8d53-ae2dc3b03746';
  const text = process.argv[2] || 'Hello Plux';
  await page.goto(`http://localhost:8081/o/${env.PLUX_DEV_ORGANIZATION}/apps/${env.PLUX_DEV_APP}/plugins/${plugin}`);
  await page.getByRole('tab', { name: 'Layers' }).waitFor({ timeout: 30000 });
  await page.waitForTimeout(4000);
  await page.getByRole('tab', { name: 'Layers' }).click();
  await page.getByRole('button', { name: 'Expand all' }).click();
  await page.waitForTimeout(500);
  await page.locator('[data-item-id="01a1c100-0000-7000-8000-000000000004"]').first().click();
  await page.waitForTimeout(800);
  const b = await overlayBox(page);
  console.log('box', JSON.stringify(b));
  await page.mouse.dblclick(b.x + b.w / 2, b.y + b.h / 2);
  await page.waitForTimeout(800);
  const ta = page.locator('textarea').first();
  console.log('inline editor', await ta.count());
  await ta.fill(text);
  await ta.press('Enter');
  await page.waitForTimeout(6000);
  await page.screenshot({ path: 's3.png' });
  summary(log);
  await browser.close();
})().catch(e => { console.error('ERR', e.message.slice(0, 400)); process.exit(1); });
