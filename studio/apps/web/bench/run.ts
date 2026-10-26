// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Overlay-open benchmark (STU-123): headless Chromium on a software renderer, Monaco's JSON editor in a
// full-screen overlay; three runs of the page, the median of each number is printed and the budget enforced.
// `make studio-bench-overlay`.

import { join, resolve } from "node:path";
import { chromium } from "@playwright/test";

/** STU-123: an overlay opens in at most this many milliseconds once its code is loaded. */
const BUDGET_MS = 150;
const RUNS = 3;
const root = import.meta.dir;

async function bundle(entry: string): Promise<{ js: string; css: string }> {
  const result = await Bun.build({
    entrypoints: [resolve(root, entry)],
    target: "browser",
    format: "esm",
    loader: { ".ttf": "dataurl" },
  });
  if (!result.success) {
    throw new Error(`bundling ${entry} failed: ${result.logs.join("\n")}`);
  }
  const text = async (kind: string, ext: string) => {
    const out = result.outputs.find((o) => o.kind === kind && o.path.endsWith(ext));
    return out === undefined ? "" : out.text();
  };
  return { js: await text("entry-point", ".js"), css: await text("asset", ".css") };
}

const monacoRoot = join(root, "../node_modules/monaco-editor/esm/vs");
const page = await bundle("overlay-page.ts");
const editorWorker = await bundle(join(monacoRoot, "editor/editor.worker.js"));
const jsonWorker = await bundle(join(monacoRoot, "language/json/json.worker.js"));
const html = `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/page.css"></head><body style="margin:0"><script type="module" src="/page.js"></script></body></html>`;

const files: Record<string, [string, string]> = {
  "/": [html, "text/html"],
  "/page.js": [page.js, "text/javascript"],
  "/page.css": [page.css, "text/css"],
  "/editor.worker.js": [editorWorker.js, "text/javascript"],
  "/json.worker.js": [jsonWorker.js, "text/javascript"],
};
const server = Bun.serve({
  port: 0,
  fetch(request) {
    const hit = files[new URL(request.url).pathname];
    return hit === undefined
      ? new Response("not found", { status: 404 })
      : new Response(hit[0], { headers: { "content-type": hit[1] } });
  },
});

function median(values: number[]): number {
  const sorted = values.slice().sort((a, b) => a - b);
  return sorted[Math.floor(sorted.length / 2)] ?? 0;
}

const browser = await chromium.launch({
  headless: true,
  args: ["--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
});
try {
  const runs: { p50: number; p95: number; max: number; lines: number }[] = [];
  for (let run = 0; run < RUNS; run++) {
    const tab = await browser.newPage({ viewport: { width: 1280, height: 800 } });
    const errors: string[] = [];
    tab.on("pageerror", (e) => errors.push(e.message));
    await tab.goto(`http://localhost:${server.port}/`);
    const result = await tab.waitForSelector("#result", { state: "attached", timeout: 120_000 });
    const parsed = JSON.parse((await result.textContent()) ?? "{}") as {
      error?: string;
      p50: number;
      p95: number;
      max: number;
      lines: number;
    };
    await tab.close();
    if (errors.length > 0 || parsed.error !== undefined) {
      process.stderr.write(`page errors: ${errors.join("; ")} ${parsed.error ?? ""}\n`);
      process.exitCode = 1;
    } else {
      runs.push(parsed);
    }
  }
  if (process.exitCode === undefined) {
    const summary = {
      runs: RUNS,
      lines: runs[0]?.lines,
      p50: median(runs.map((r) => r.p50)),
      p95: median(runs.map((r) => r.p95)),
      max: median(runs.map((r) => r.max)),
      budgetMs: BUDGET_MS,
    };
    process.stdout.write(`${JSON.stringify(summary, null, 2)}\n`);
    if (summary.p95 > BUDGET_MS) {
      process.stderr.write(`STU-123: p95 ${summary.p95} ms exceeds ${BUDGET_MS} ms\n`);
      process.exitCode = 1;
    }
  }
} finally {
  await browser.close();
  server.stop();
}
