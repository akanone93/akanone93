// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { readFileSync } from "node:fs";

/** One entry of the manifest that Vite writes with `build.manifest`. */
export type ManifestChunk = {
  file: string;
  isEntry?: boolean;
  imports?: string[];
  dynamicImports?: string[];
};

export type Manifest = Record<string, ManifestChunk>;

export type SizedFile = { file: string; gzipBytes: number };

/** Returns the files of the entry chunk and its static imports, each once. */
export function initialFiles(manifest: Manifest): string[] {
  const seen = new Set<string>();
  const files: string[] = [];
  const visit = (key: string): void => {
    const chunk = manifest[key];
    if (chunk === undefined || seen.has(key)) {
      return;
    }
    seen.add(key);
    if (chunk.file.endsWith(".js")) {
      files.push(chunk.file);
    }
    for (const imported of chunk.imports ?? []) {
      visit(imported);
    }
  };
  for (const [key, chunk] of Object.entries(manifest)) {
    if (chunk.isEntry === true) {
      visit(key);
    }
  }
  return files;
}

/** Sums the gzip sizes of the initial JavaScript and compares it with the budget. */
export function measure(
  manifest: Manifest,
  read: (file: string) => Uint8Array<ArrayBuffer>,
  budget: number,
): { files: SizedFile[]; total: number; withinBudget: boolean } {
  const files = initialFiles(manifest).map((file) => ({ file, gzipBytes: Bun.gzipSync(read(file)).length }));
  const total = files.reduce((sum, f) => sum + f.gzipBytes, 0);
  return { files, total, withinBudget: total <= budget };
}

async function main(): Promise<number> {
  const dist = new URL("../apps/web/dist/", import.meta.url);
  const manifest = (await Bun.file(new URL(".vite/manifest.json", dist)).json()) as Manifest;
  const budgets = (await Bun.file(new URL("../budgets.json", import.meta.url)).json()) as {
    initialJsGzipBytes: number;
  };
  const result = measure(manifest, (file) => readFileSync(new URL(file, dist)), budgets.initialJsGzipBytes);
  for (const f of result.files) {
    process.stdout.write(`${String(f.gzipBytes).padStart(10)}  ${f.file}\n`);
  }
  process.stdout.write(
    `${String(result.total).padStart(10)}  total initial JS gzip (budget ${budgets.initialJsGzipBytes})\n`,
  );
  return result.withinBudget ? 0 : 1;
}

if (import.meta.main) {
  process.exit(await main());
}
