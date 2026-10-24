// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type LimitKey, limits } from "./limits.gen.ts";

/**
 * The registry default of a limit (LIM-001). Studio never hard-codes a size or count: it reads it here,
 * and the key is checked against the generated registry at compile time.
 */
export function limitDefault(key: LimitKey): number {
  const found = limits.find((l) => l.key === key);
  if (found === undefined) {
    throw new Error(`unknown limit key ${key}`);
  }
  return found.default;
}
