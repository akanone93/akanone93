// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Measures the no-GPU techniques (M-12) in headless Chromium under software GL.
// Usage: bun run nogpu-measure.ts   (prints a JSON array of rows to stdout)

import { chromium } from "playwright";
import { resolvedSource, startServer } from "./serve";

const server = await startServer(resolvedSource("flutter"));
const browser = await chromium.launch({
  headless: true,
  args: ["--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
});
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
  page.on("pageerror", (error) => console.error(`page error: ${error.message}`));
  const rows: unknown[] = [];
  for (const renderer of ["gl", "sw"]) {
    await page.goto(`${server.url}/`);
    const r = (await page.evaluate(
      (name) => (window as unknown as { plux: { runNoGpu(n: string): Promise<unknown> } }).plux.runNoGpu(name),
      renderer,
    )) as { type: string; reason?: string; results?: unknown[] };
    if (r.type !== "result") {
      console.error(`${renderer}: ${r.reason}`);
      continue;
    }
    rows.push(...(r.results ?? []));
  }
  process.stdout.write(`${JSON.stringify(rows)}\n`);
} finally {
  await browser.close();
  server.stop();
}
