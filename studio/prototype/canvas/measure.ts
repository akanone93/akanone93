// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Runs the whole measurement in headless Chromium and prints JSON to stdout.
// Usage: bun run measure.ts [--canvaskit=flutter|flutter-chromium|npm|npm-full] [--only=validator]

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { chromium } from "playwright";
import { type CanvasKitSource, canvasKitDir, dist, repoRoot, resolvedSource, startServer } from "./serve";

function log(message: string): void {
  // biome-ignore lint/suspicious/noConsole: measurement progress goes to stderr
  console.error(message);
}

function size(path: string): { path: string; bytes: number; gzipBytes: number } {
  const data = new Uint8Array(readFileSync(path));
  return { path, bytes: data.length, gzipBytes: Bun.gzipSync(data, { level: 9 }).length };
}

async function sh(cmd: string[], cwd: string, env: Record<string, string> = {}): Promise<string> {
  const proc = Bun.spawn(cmd, { cwd, env: { ...process.env, ...env }, stdout: "pipe", stderr: "inherit" });
  const out = await new Response(proc.stdout).text();
  if ((await proc.exited) !== 0) {
    throw new Error(`${cmd.join(" ")} failed`);
  }
  return out.trim();
}

async function buildValidator(): Promise<void> {
  mkdirSync(dist, { recursive: true });
  const goroot = await sh(["go", "env", "GOROOT"], join(repoRoot, "backend"));
  const execJs = [join(goroot, "lib/wasm/wasm_exec.js"), join(goroot, "misc/wasm/wasm_exec.js")].find(existsSync);
  if (!execJs) {
    throw new Error("wasm_exec.js not found in GOROOT");
  }
  await Bun.write(join(dist, "wasm_exec.js"), Bun.file(execJs));
  await sh(
    ["go", "build", "-trimpath", "-ldflags=-s -w", "-o", join(dist, "validator.wasm"), "./cmd/plux-validator-wasm"],
    join(repoRoot, "backend"),
    { GOOS: "js", GOARCH: "wasm" },
  );
}

function cpuModel(): string {
  const text = readFileSync("/proc/cpuinfo", "utf8");
  const model = /model name\s*:\s*(.*)/.exec(text)?.[1] ?? "unknown";
  const cores = (text.match(/^processor/gm) ?? []).length;
  return `${model} (${cores} logical CPUs)`;
}

const arg = process.argv.find((a) => a.startsWith("--canvaskit="))?.split("=")[1];
const requested = (arg ?? "flutter") as CanvasKitSource;
const source = resolvedSource(requested);

log("building the validator WebAssembly module");
await buildValidator();
const server = await startServer(source);
const browser = await chromium.launch({
  headless: true,
  args: ["--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
});
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
  page.on("pageerror", (error) => log(`page error: ${error.message}`));
  page.on("console", (msg) => {
    if (msg.type() === "error") {
      log(`browser console error: ${msg.text()}`);
    }
  });
  const run = async (options: { scenes: string[]; forceMainThread: boolean }): Promise<unknown> => {
    await page.goto(`${server.url}/`);
    return page.evaluate((o) => (window as unknown as { plux: { runCanvas(o: unknown): Promise<unknown> } }).plux.runCanvas(o), options);
  };
  const canvasScenes = process.argv.includes("--only=validator") ? [] : ["A", "B"];
  log("canvas scenes A and B in a worker");
  const worker = canvasScenes.length ? await run({ scenes: canvasScenes, forceMainThread: false }) : null;
  log("scene A on the main thread, for comparison");
  const mainThread = canvasScenes.length ? await run({ scenes: ["A"], forceMainThread: true }) : null;
  log("ValidatePage in a worker");
  await page.goto(`${server.url}/`);
  const validator = await page.evaluate(() =>
    (window as unknown as { plux: { validate(n: number): Promise<unknown> } }).plux.validate(50),
  );
  const ckDir = canvasKitDir(source);
  const results = {
    environment: {
      cpu: cpuModel(),
      browser: `Chromium ${browser.version()} (headless, software GL via SwiftShader, no GPU)`,
      canvasKitSource: source,
      canvasKitSourceRequested: requested,
      canvasKitDir: ckDir,
      trimmedCanvasKitBuild: "blocked by network policy (skia.googlesource.com, emsdk)",
    },
    sizes: {
      canvaskitJs: size(join(ckDir, "canvaskit.js")),
      canvaskitWasm: size(join(ckDir, "canvaskit.wasm")),
      validatorWasm: size(join(dist, "validator.wasm")),
    },
    worker,
    mainThread,
    validator,
  };
  writeFileSync(join(dist, "results.json"), JSON.stringify(results, null, 2));
  process.stdout.write(`${JSON.stringify(results, null, 2)}\n`);
} finally {
  await browser.close();
  server.stop();
}
