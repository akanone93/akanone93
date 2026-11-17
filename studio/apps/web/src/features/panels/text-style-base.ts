// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { valueTypes } from "@plux/schema";

/**
 * A text style that starts from a typography token and changes some of its fields (build log #84): `{"base":
 * {"$token": …}, "color": …}`, which the device merges as Flutter's `TextStyle.merge` does. The `base` field arrived in
 * a later revision of TextStyle, so only an app whose oldest runtime understands it may use it (WGT-004).
 */

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** A runtime version as numbers, for ordering. */
const versionParts = (v: string): number[] => v.split(".").map((x) => Number.parseInt(x, 10) || 0);

/** Whether runtime `have` is `want` or newer. */
function atLeast(have: string, want: string): boolean {
  const [a, b] = [versionParts(have), versionParts(want)];
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    if ((a[i] ?? 0) !== (b[i] ?? 0)) {
      return (a[i] ?? 0) > (b[i] ?? 0);
    }
  }
  return true;
}

/** The first runtime that understands a TextStyle's `base`, from the registry. */
export function baseRuntime(): string | undefined {
  const type = valueTypes.find((t) => t.name === "TextStyle");
  const field = type?.fields.find((f) => f.name === "base");
  return field === undefined ? undefined : type?.runtimes[field.revision - 1];
}

/** Whether an app whose oldest runtime is `pin` may write a style that starts from a token. */
export function canExtend(pin: string | undefined): boolean {
  const runtime = baseRuntime();
  return pin !== undefined && runtime !== undefined && atLeast(pin, runtime);
}

/** The typography token a style is or starts from, if any. */
export function baseTokenOf(value: unknown): string | undefined {
  if (!isObj(value)) {
    return undefined;
  }
  if (typeof value["$token"] === "string") {
    return value["$token"];
  }
  const base = value["base"];
  return isObj(base) && typeof base["$token"] === "string" ? base["$token"] : undefined;
}

/** The fields a style sets itself, over its base or token. */
export function overridesOf(value: unknown): Obj {
  if (!isObj(value) || typeof value["$token"] === "string") {
    return {};
  }
  const { base: _base, ...own } = value;
  return own;
}

/** A style of typography token `token` with `overrides`: the token alone when nothing is changed. */
export function extendToken(token: string, overrides: Obj): Obj {
  return Object.keys(overrides).length === 0 ? { $token: token } : { base: { $token: token }, ...overrides };
}
