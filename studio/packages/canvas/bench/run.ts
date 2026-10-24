// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Canvas benchmark in software rendering (STU-045, STU-120, ADR-0013 M-12):
// headless Chromium with SwiftShader (no GPU), the pinned Flutter CanvasKit,
// scenes A and B; prints a JSON summary. `make studio-bench-canvas`.

import { join } from "node:path";
import { chromium } from "@playwright/test";

const root = import.meta.dir;
const canvasKitDir =
  process.env.PLUX_CANVASKIT_DIR ?? "/root/.cache/plux/flutter-3.47.5/bin/cache/flutter_web_sdk/canvaskit";
const font = join(root, "../../../../schema/goldens/fonts/NotoSans_400Regular.ttf");

async function bundle(entry: string): Promise<string> {
  const result = await Bun.build({ entrypoints: [join(root, entry)], target: "browser", format: "esm" });
  const [output] = result.outputs;
  if (!result.success || !output) {
    throw new Error(`bundling ${entry} failed: ${result.logs.join("\n")}`);
  }
  return output.text();
}

const scripts: Record<string, string> = {
  "/page.js": await bundle("page.ts"),
  "/worker.js": await bundle("worker-entry.ts"),
};
const html =
  '<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0"><script type="module" src="/page.js"></script></body></html>';
// Cross-origin isolation gives the worker a fine-grained performance.now().
const isolation = {
  "cross-origin-opener-policy": "same-origin",
  "cross-origin-embedder-policy": "require-corp",
  "cross-origin-resource-policy": "same-origin",
};

function route(request: Request): Response {
  const { pathname } = new URL(request.url);
  const script = scripts[pathname];
  if (script !== undefined) {
    return new Response(script, { headers: { ...isolation, "content-type": "text/javascript" } });
  }
  if (pathname === "/") {
    return new Response(html, { headers: { ...isolation, "content-type": "text/html" } });
  }
  if (pathname === "/font.ttf") {
    return new Response(Bun.file(font), { headers: isolation });
  }
  const name = pathname.startsWith("/canvaskit/") ? pathname.slice("/canvaskit/".length) : "";
  if (/^canvaskit\.(js|wasm)$/.test(name)) {
    const type = name.endsWith(".wasm") ? "application/wasm" : "text/javascript";
    return new Response(Bun.file(join(canvasKitDir, name)), { headers: { ...isolation, "content-type": type } });
  }
  return new Response("not found", { status: 404 });
}

const server = Bun.serve({ port: 0, fetch: route });
const browser = await chromium.launch({
  headless: true,
  args: ["--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
});
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const renderer = process.env.PLUX_CANVAS_RENDERER ?? "auto";
  await page.goto(`http://localhost:${server.port}/?renderer=${encodeURIComponent(renderer)}`);
  const result = await page.waitForSelector("#result", { state: "attached", timeout: 600_000 });
  const text = (await result.textContent()) ?? "";
  process.stdout.write(`${text}\n`);
  if (errors.length > 0 || text.includes('"error"')) {
    process.stderr.write(`page errors: ${errors.join("; ")}\n`);
    process.exitCode = 1;
  }
} finally {
  await browser.close();
  server.stop();
}
