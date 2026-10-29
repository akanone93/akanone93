// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Largest Contentful Paint of the built app on a typical broadband connection (STU-122): headless
// Chromium loads apps/web/dist cold (no cache, a new context per run) through a server that answers
// like the BFF does (Brotli siblings, the sign-in page when the API says 401), over emulated
// 10 Mbit/s down, 5 Mbit/s up and 40 ms round trip. No CPU throttling and no GPU. The median of RUNS
// runs is compared with the budget in budgets.json. `make studio-bench-lcp`.

import { existsSync } from "node:fs";
import { extname, join, normalize, sep } from "node:path";
import { chromium } from "@playwright/test";

/** Emulated connection: "typical broadband" for STU-122. */
export const CONNECTION = { downMbit: 10, upMbit: 5, latencyMs: 40 } as const;
const RUNS = 5;

const types: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript",
  ".mjs": "text/javascript",
  ".css": "text/css",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".wasm": "application/wasm",
  ".woff2": "font/woff2",
  ".woff": "font/woff",
  ".ttf": "font/ttf",
  ".png": "image/png",
  ".txt": "text/plain",
  ".webmanifest": "application/manifest+json",
};

/** The median of a non-empty list of numbers. */
export function median(values: number[]): number {
  const sorted = values.slice().sort((a, b) => a - b);
  return sorted[Math.floor(sorted.length / 2)] ?? 0;
}

/** Serves `dist`: a file as it is, with its Brotli sibling when the client accepts it; the API says 401; anything else is the app shell. */
export function serve(dist: string, request: Request): Response {
  const { pathname } = new URL(request.url);
  if (pathname.startsWith("/api/")) {
    return new Response(JSON.stringify({ code: "unauthenticated", message: "no session" }), {
      status: 401,
      headers: { "content-type": "application/json" },
    });
  }
  const relative = normalize(pathname === "/" ? "index.html" : pathname.slice(1));
  const candidate = join(dist, relative);
  const inside = candidate.startsWith(dist + sep);
  const path = inside && existsSync(candidate) ? candidate : join(dist, "index.html");
  const headers: Record<string, string> = {
    "content-type": types[extname(path)] ?? "application/octet-stream",
    "cache-control": "no-store",
  };
  if ((request.headers.get("accept-encoding") ?? "").includes("br") && existsSync(`${path}.br`)) {
    headers["content-encoding"] = "br";
    headers["vary"] = "accept-encoding";
    return new Response(Bun.file(`${path}.br`), { headers });
  }
  return new Response(Bun.file(path), { headers });
}

async function main(): Promise<number> {
  const dist = join(import.meta.dir, "../apps/web/dist");
  if (!existsSync(join(dist, "index.html"))) {
    process.stderr.write(`no build in ${dist}: run 'make studio-bench-lcp'\n`);
    return 1;
  }
  const budgets = (await Bun.file(join(import.meta.dir, "../budgets.json")).json()) as {
    largestContentfulPaintMs: number;
  };
  const server = Bun.serve({ port: 0, fetch: (request) => serve(dist, request) });
  const browser = await chromium.launch({ headless: true });
  try {
    const samples: number[] = [];
    for (let run = 0; run < RUNS; run++) {
      const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
      const page = await context.newPage();
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      await page.addInitScript(() => {
        const seen: number[] = [];
        (window as unknown as { __lcp: number[] }).__lcp = seen;
        new PerformanceObserver((list) => {
          for (const entry of list.getEntries()) {
            seen.push(entry.startTime);
          }
        }).observe({ type: "largest-contentful-paint", buffered: true });
      });
      const cdp = await context.newCDPSession(page);
      await cdp.send("Network.enable");
      await cdp.send("Network.emulateNetworkConditions", {
        offline: false,
        latency: CONNECTION.latencyMs,
        downloadThroughput: (CONNECTION.downMbit * 1_000_000) / 8,
        uploadThroughput: (CONNECTION.upMbit * 1_000_000) / 8,
      });
      await page.goto(`http://127.0.0.1:${server.port}/`, { waitUntil: "networkidle" });
      await page.waitForSelector("main", { state: "attached" });
      await page.waitForTimeout(500);
      const seen = await page.evaluate(() => (window as unknown as { __lcp: number[] }).__lcp);
      await context.close();
      const last = seen[seen.length - 1];
      if (errors.length > 0 || last === undefined) {
        process.stderr.write(`run ${run}: ${errors.join("; ") || "no largest-contentful-paint entry"}\n`);
        return 1;
      }
      samples.push(Math.round(last));
    }
    const lcp = median(samples);
    process.stdout.write(
      `${JSON.stringify({ runs: RUNS, connection: CONNECTION, samplesMs: samples, medianMs: lcp, budgetMs: budgets.largestContentfulPaintMs }, null, 2)}\n`,
    );
    if (lcp > budgets.largestContentfulPaintMs) {
      process.stderr.write(`STU-122: LCP ${lcp} ms exceeds ${budgets.largestContentfulPaintMs} ms\n`);
      return 1;
    }
    return 0;
  } finally {
    await browser.close();
    server.stop();
  }
}

if (import.meta.main) {
  process.exit(await main());
}
