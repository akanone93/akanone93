// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// A tiny static server for the prototype: bundles the page and worker
// scripts on start, and serves CanvasKit, the font, the starter pages and the
// validator WebAssembly module from disk.

import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";

export const root = import.meta.dir;
export const repoRoot = resolve(root, "../../..");
export const dist = join(root, "dist");
const starter = join(repoRoot, "schema/testdata/documents/starter");

export type CanvasKitSource = "flutter" | "flutter-chromium" | "npm" | "npm-full";

function flutterCanvasKit(): string | undefined {
  const base = join(process.env.HOME ?? "/root", ".cache/plux");
  if (!existsSync(base)) {
    return undefined;
  }
  for (const entry of readdirSync(base)) {
    const dir = join(base, entry, "bin/cache/flutter_web_sdk/canvaskit");
    if (existsSync(join(dir, "canvaskit.wasm"))) {
      return dir;
    }
  }
  return undefined;
}

export function canvasKitDir(source: CanvasKitSource): string {
  const npm = join(root, "node_modules/canvaskit-wasm/bin");
  const flutter = flutterCanvasKit();
  switch (source) {
    case "flutter":
      return flutter ?? npm;
    case "flutter-chromium":
      return flutter ? join(flutter, "chromium") : npm;
    case "npm-full":
      return join(npm, "full");
    default:
      return npm;
  }
}

export function resolvedSource(source: CanvasKitSource): CanvasKitSource {
  return source.startsWith("flutter") && !flutterCanvasKit() ? "npm" : source;
}

function fontPath(): string {
  const override = process.env.PLUX_FONT;
  if (override) {
    return override;
  }
  const dir = flutterCanvasKit();
  const roboto = dir ? join(dir, "../../artifacts/material_fonts/Roboto-Regular.ttf") : "";
  if (!existsSync(roboto)) {
    throw new Error("set PLUX_FONT to a TTF file (the Flutter material_fonts Roboto-Regular.ttf is used when present)");
  }
  return roboto;
}

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir).sort()) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) {
      walk(path, out);
    } else {
      out.push(path);
    }
  }
  return out;
}

/** The whole starter project, as the validator's files argument. */
function projectFiles(): unknown[] {
  return walk(starter).map((path) => {
    const bytes = readFileSync(path);
    const binary = path.endsWith(".png");
    return {
      path: relative(starter, path).split("\\").join("/"),
      data: binary ? bytes.toString("base64") : bytes.toString("utf8"),
      ...(binary ? { encoding: "base64" } : {}),
    };
  });
}

function pages(): unknown[] {
  const dir = join(starter, "plugins/welcome/pages");
  return readdirSync(dir)
    .filter((name) => name.endsWith(".page.json"))
    .sort()
    .map((name) => JSON.parse(readFileSync(join(dir, name), "utf8")));
}

async function bundle(entry: string, format: "iife" | "esm"): Promise<string> {
  const result = await Bun.build({ entrypoints: [join(root, entry)], target: "browser", format });
  if (!result.success) {
    throw new Error(`bundling ${entry} failed: ${result.logs.join("\n")}`);
  }
  const [output] = result.outputs;
  if (!output) {
    throw new Error(`bundling ${entry} produced no output`);
  }
  return output.text();
}

export async function startServer(source: CanvasKitSource): Promise<{ url: string; stop(): void }> {
  const scripts: Record<string, string> = {
    "/main.js": await bundle("main.ts", "iife"),
    "/worker.js": await bundle("worker.ts", "esm"),
    "/nogpu-worker.js": await bundle("nogpu-worker.ts", "esm"),
    "/validator-worker.js": await bundle("validator-worker.ts", "iife"),
  };
  const ckDir = canvasKitDir(source);
  const files: Record<string, string> = {
    "/": join(root, "index.html"),
    "/font.ttf": fontPath(),
    "/validator.wasm": join(dist, "validator.wasm"),
    "/wasm_exec.js": join(dist, "wasm_exec.js"),
  };
  const projectJson = JSON.stringify(projectFiles());
  const pagesJson = JSON.stringify(pages());
  // Cross-origin isolation gives workers a fine-grained performance.now().
  const isolation = {
    "cross-origin-opener-policy": "same-origin",
    "cross-origin-embedder-policy": "require-corp",
    "cross-origin-resource-policy": "same-origin",
  };
  const route = (request: Request): Response => {
    const { pathname } = new URL(request.url);
    const script = scripts[pathname];
    if (script !== undefined) {
      return new Response(script, { headers: { "content-type": "text/javascript" } });
    }
    if (pathname === "/starter.json") {
      return new Response(projectJson, { headers: { "content-type": "application/json" } });
    }
    if (pathname === "/pages.json") {
      return new Response(pagesJson, { headers: { "content-type": "application/json" } });
    }
    const ckName = pathname.startsWith("/canvaskit/") ? pathname.slice("/canvaskit/".length) : "";
    const path = ckName && !ckName.includes("/") ? join(ckDir, ckName) : files[pathname];
    if (path && existsSync(path)) {
      if (ckName === "canvaskit.js") {
        // The npm build is a UMD script; the Flutter build is already an ES module.
        const text = readFileSync(path, "utf8");
        const module = text.includes("export default") ? text : `${text}\nexport default CanvasKitInit;\n`;
        return new Response(module, { headers: { "content-type": "text/javascript" } });
      }
      return new Response(Bun.file(path));
    }
    return new Response("not found", { status: 404 });
  };
  const server = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    fetch(request) {
      const response = route(request);
      for (const [key, value] of Object.entries(isolation)) {
        response.headers.set(key, value);
      }
      return response;
    },
  });
  return { url: `http://127.0.0.1:${server.port}`, stop: () => void server.stop(true) };
}

if (import.meta.main) {
  const server = await startServer((process.argv[2] as CanvasKitSource | undefined) ?? "flutter");
  // biome-ignore lint/suspicious/noConsole: measurement output
  console.log(`serving ${server.url}`);
}
