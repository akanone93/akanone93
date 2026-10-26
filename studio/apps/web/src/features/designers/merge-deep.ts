// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { jsonEqual } from "../editor/json-patch.ts";

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

/**
 * Makes `draft` equal `next` with the fewest changes, looking inside arrays too: entries that are equal stay
 * untouched, entries at the same position are merged, and entries past the shorter length are added or removed.
 * A designer that edits one field of one data source therefore saves one patch operation, not the whole list.
 */
export function mergeDeep(draft: Record<string, unknown>, next: Record<string, unknown>): void {
  for (const key of Object.keys(draft)) {
    if (!(key in next)) {
      delete draft[key];
    }
  }
  for (const [key, value] of Object.entries(next)) {
    const current = draft[key];
    if (isObject(current) && isObject(value)) {
      mergeDeep(current, value);
    } else if (Array.isArray(current) && Array.isArray(value)) {
      mergeList(current, value);
    } else if (!jsonEqual(current, value)) {
      draft[key] = value;
    }
  }
}

function mergeList(draft: unknown[], next: readonly unknown[]): void {
  const common = Math.min(draft.length, next.length);
  for (let i = 0; i < common; i += 1) {
    const current = draft[i];
    const value = next[i];
    if (isObject(current) && isObject(value)) {
      mergeDeep(current, value);
    } else if (Array.isArray(current) && Array.isArray(value)) {
      mergeList(current, value);
    } else if (!jsonEqual(current, value)) {
      draft[i] = value;
    }
  }
  if (draft.length > next.length) {
    draft.length = next.length;
  }
  for (let i = common; i < next.length; i += 1) {
    draft.push(next[i]);
  }
}
