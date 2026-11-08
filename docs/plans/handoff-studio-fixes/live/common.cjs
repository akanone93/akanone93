// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0
const { chromium } = require('playwright');
const fs = require('fs');
const env = Object.fromEntries(fs.readFileSync('/var/tmp/plux-dev/dev.env','utf8').trim().split('\n').map(l => l.split(/=(.*)/s).slice(0,2)));
async function open() {
  const browser = await chromium.launch({ args: ["--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"] });
  const page = await browser.newPage({ viewport: { width: 1600, height: 1000 } });
  const log = { console: [], failed: [] };
  page.on('console', m => { if (m.type() === 'error' || m.type() === 'warning') log.console.push(`[${m.type()}] ${m.text().slice(0, 300)}`); });
  page.on('pageerror', e => log.console.push(`[pageerror] ${String(e).slice(0, 300)}`));
  page.on('response', async r => {
    if (r.status() >= 400 && r.url().includes('/api/')) {
      let body = ''; try { body = (await r.text()).slice(0, 400); } catch {}
      let req = ''; try { req = (r.request().postData() || '').slice(0, 300); } catch {}
      log.failed.push(`${r.status()} ${r.url().replace(/.*\/api\//,'')} :: ${body} :: req ${req}`);
    }
  });
  await page.goto('http://localhost:8081/signin');
  await page.getByLabel(/email/i).fill('dev@plux.localhost');
  await page.getByLabel(/^password$/i).fill(env.PLUX_DEV_PASSWORD);
  await page.getByRole('button', { name: /sign in/i }).click();
  await page.waitForTimeout(3000);
  return { browser, page, log, env };
}
function summary(log) {
  const count = {}; for (const c of log.console) { const k = c.slice(0, 140); count[k] = (count[k] || 0) + 1; }
  console.log('--- console (deduped) ---'); for (const [k, v] of Object.entries(count)) console.log(v + 'x ' + k);
  console.log('--- failed api ---'); const f = {}; for (const c of log.failed) { const k = c.slice(0, 600); f[k] = (f[k] || 0) + 1; } for (const [k, v] of Object.entries(f)) console.log(v + 'x ' + k);
}
module.exports = { open, summary };
