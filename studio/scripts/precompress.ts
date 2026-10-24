// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { brotliCompressSync, constants } from "node:zlib";

const compressible = /\.(js|mjs|css|html|svg|json|wasm|txt|map|webmanifest)$/;

/** Writes a Brotli (quality 11) sibling for every compressible file under dir; returns the count. */
export function precompress(dir: string): number {
  let n = 0;
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) {
      n += precompress(path);
    } else if (compressible.test(name)) {
      const data = readFileSync(path);
      const out = brotliCompressSync(data, {
        params: { [constants.BROTLI_PARAM_QUALITY]: 11, [constants.BROTLI_PARAM_SIZE_HINT]: data.length },
      });
      writeFileSync(`${path}.br`, out);
      n += 1;
    }
  }
  return n;
}

if (import.meta.main) {
  const dir = process.argv[2] ?? "apps/web/dist";
  process.stdout.write(`precompress: ${precompress(dir)} files in ${dir}\n`);
}
