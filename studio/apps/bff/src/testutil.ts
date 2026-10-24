// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { precompress } from "../../../scripts/precompress.ts";

/** Runs the build step's compressor on a directory; used to cover it in tests. */
export function precompressForTest(dir: string): number {
  return precompress(dir);
}
