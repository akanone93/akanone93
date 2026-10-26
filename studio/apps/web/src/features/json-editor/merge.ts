// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { jsonEqual } from "../editor/json-patch.ts";

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

/**
 * Makes the object `draft` equal `next` with the fewest changes: members that are equal stay untouched, so the
 * edit pipeline records (and saves) a JSON Patch of only what the user changed, not a rewrite of the document.
 */
export function mergeInto(draft: Record<string, unknown>, next: Record<string, unknown>): void {
  for (const key of Object.keys(draft)) {
    if (!(key in next)) {
      delete draft[key];
    }
  }
  for (const [key, value] of Object.entries(next)) {
    const current = draft[key];
    if (isObject(current) && isObject(value)) {
      mergeInto(current, value);
    } else if (!jsonEqual(current, value)) {
      draft[key] = value;
    }
  }
}
