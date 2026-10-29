// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { limitDefault } from "@plux/schema";
import { DARK_KEY, flattenTokens, type TokenRow } from "../designers/theme-overlay.tsx";
import { isObject, type Json } from "../designers/use-document.ts";

/** The key under which a token carries its high-contrast value (THM-001, STU-112). */
export const CONTRAST_KEY = "dev.plux.highContrast";

export type Mode = "light" | "dark" | "contrast";

/** The scripts whose font families the theme defines with fallbacks (THM-004). */
export const SCRIPTS = ["latin", "cyrillic", "arabic", "devanagari", "ethiopic"] as const;

/** The token types of the theme schema (THM-001); an import never adds another. */
export const TOKEN_TYPES: ReadonlySet<string> = new Set([
  "color",
  "dimension",
  "fontFamily",
  "fontWeight",
  "duration",
  "cubicBezier",
  "number",
  "typography",
  "shadow",
  "border",
  "strokeStyle",
  "gradient",
  "transition",
]);

const NAME = /^[a-zA-Z0-9][a-zA-Z0-9-]*$/;

/** A token name is dot-separated segments that the theme schema accepts. */
export const validName = (name: string): boolean => name !== "" && name.split(".").every((s) => NAME.test(s));

const GROUPS = ["typography", "icons", "brand"] as const;

/** The base design tokens: everything that is not typography, an icon set or a brand overlay. */
export const baseRows = (rows: readonly TokenRow[]): TokenRow[] =>
  rows.filter((r) => !(GROUPS as readonly string[]).includes(r.path[0] ?? ""));

/** Rows below one top-level group, with the group name taken off their path. */
export const groupRows = (rows: readonly TokenRow[], group: (typeof GROUPS)[number]): TokenRow[] =>
  rows.filter((r) => r.path[0] === group).map((r) => ({ ...r, path: r.path.slice(1) }));

/** The names of the brand overlays: the second path segment under `brand`. */
export const brandNames = (rows: readonly TokenRow[]): string[] => [
  ...new Set(groupRows(rows, "brand").flatMap((r) => (r.path[0] === undefined ? [] : [r.path[0]]))),
];

/** The font families of a `fontFamily` value: a list, or a single name. */
export const families = (value: unknown): string[] =>
  Array.isArray(value)
    ? value.filter((v): v is string => typeof v === "string")
    : typeof value === "string"
      ? [value]
      : [];

/** Whether every script of THM-004 has a family with at least one fallback. */
export function scriptCoverage(rows: readonly TokenRow[]): Record<(typeof SCRIPTS)[number], number> {
  const typography = groupRows(rows, "typography");
  const count = (script: string) => typography.find((r) => r.path[0] === script)?.light;
  return Object.fromEntries(SCRIPTS.map((s) => [s, families(count(s)).length])) as Record<
    (typeof SCRIPTS)[number],
    number
  >;
}

function leafAt(tokens: Json, path: readonly string[]): Json | undefined {
  let node: unknown = tokens;
  for (const key of path) {
    node = isObject(node) ? node[key] : undefined;
  }
  return isObject(node) ? node : undefined;
}

/** Sets or clears (`undefined`) the value of one mode of an existing token. */
export function setMode(tokens: Json, path: readonly string[], mode: Mode, value: unknown): void {
  const leaf = leafAt(tokens, path);
  if (leaf === undefined) {
    return;
  }
  if (mode === "light") {
    leaf["$value"] = value;
    return;
  }
  const key = mode === "dark" ? DARK_KEY : CONTRAST_KEY;
  const ext: Json = isObject(leaf["$extensions"]) ? leaf["$extensions"] : {};
  if (value === undefined) {
    delete ext[key];
  } else {
    ext[key] = value;
  }
  if (Object.keys(ext).length === 0) {
    delete leaf["$extensions"];
  } else {
    leaf["$extensions"] = ext;
  }
}

/** The high-contrast value of a token, read from its extensions. */
export function contrastOf(tokens: unknown, path: readonly string[]): unknown {
  const ext = isObject(tokens) ? leafAt(tokens, path)?.["$extensions"] : undefined;
  return isObject(ext) ? ext[CONTRAST_KEY] : undefined;
}

/** Adds a token at `path`, creating the groups on the way; an existing token keeps its other fields. */
export function putToken(tokens: Json, path: readonly string[], value: unknown, type?: string): void {
  let node: Json = tokens;
  for (const key of path.slice(0, -1)) {
    const child = node[key];
    if (isObject(child)) {
      node = child;
    } else {
      const made: Json = {};
      node[key] = made;
      node = made;
    }
  }
  const last = path.at(-1);
  if (last === undefined) {
    return;
  }
  const leaf = isObject(node[last]) ? node[last] : {};
  leaf["$value"] = value;
  if (type !== undefined) {
    leaf["$type"] = type;
  }
  node[last] = leaf;
}

/** Removes the token at `path`, and the groups it leaves empty. */
export function dropToken(tokens: Json, path: readonly string[]): void {
  const parent = leafAt(tokens, path.slice(0, -1));
  const last = path.at(-1);
  if (parent !== undefined && last !== undefined) {
    delete parent[last];
  }
}

/** Merges imported groups into the tokens: groups are merged, tokens replaced. */
export function mergeTokens(into: Json, from: Json): void {
  for (const [key, value] of Object.entries(from)) {
    const existing = into[key];
    if (isObject(value) && !("$value" in value) && isObject(existing) && !("$value" in existing)) {
      mergeTokens(existing, value);
    } else {
      into[key] = value;
    }
  }
}

export interface Skipped {
  readonly path: string;
  readonly reason: "name" | "type" | "value" | "depth";
}

export type DtcgResult =
  | { readonly ok: true; readonly tokens: Json; readonly imported: number; readonly skipped: Skipped[] }
  | { readonly ok: false; readonly error: "tooLarge" | "notJson" | "notObject" };

function validValue(type: string, value: unknown): boolean {
  switch (type) {
    case "color":
      return typeof value === "string" || isObject(value);
    case "number":
      return typeof value === "number";
    case "fontWeight":
      return typeof value === "number" || typeof value === "string";
    case "fontFamily":
      return typeof value === "string" || (Array.isArray(value) && value.every((v) => typeof v === "string"));
    default:
      return value !== undefined && value !== null;
  }
}

interface Walk {
  readonly skipped: Skipped[];
  imported: number;
  readonly maxDepth: number;
}

function leaf(node: Json, type: string, path: string, state: Walk): Json {
  if (!TOKEN_TYPES.has(type)) {
    state.skipped.push({ path, reason: "type" });
    return {};
  }
  if (!validValue(type, node["$value"])) {
    state.skipped.push({ path, reason: "value" });
    return {};
  }
  state.imported += 1;
  const ext = node["$extensions"];
  const dark = isObject(ext) ? ext[DARK_KEY] : undefined;
  return { $value: node["$value"], $type: type, ...(dark !== undefined && { $extensions: { [DARK_KEY]: dark } }) };
}

function walk(node: Json, path: string[], inherited: string, state: Walk): Json {
  const type = typeof node["$type"] === "string" ? node["$type"] : inherited;
  if ("$value" in node) {
    return leaf(node, type, path.join("."), state);
  }
  const out: Json = {};
  for (const [key, child] of Object.entries(node)) {
    if (key.startsWith("$") || !isObject(child)) {
      continue;
    }
    const at = [...path, key];
    const reason = !NAME.test(key) ? "name" : at.length > state.maxDepth ? "depth" : undefined;
    if (reason !== undefined) {
      state.skipped.push({ path: at.join("."), reason });
      continue;
    }
    const got = walk(child, at, type, state);
    if (Object.keys(got).length > 0) {
      out[key] = got;
    }
  }
  return out;
}

/**
 * Reads a W3C design tokens (DTCG) file, as Figma variable exports produce it. The text is untrusted: it is only
 * parsed as data, capped by the registry's document size, and every token must have a name and type the theme
 * schema accepts. What is left out is reported, never silently dropped (STU-112).
 */
export function parseDtcg(text: string): DtcgResult {
  if (new TextEncoder().encode(text).length > limitDefault("document.fileSize")) {
    return { ok: false, error: "tooLarge" };
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    return { ok: false, error: "notJson" };
  }
  if (!isObject(parsed)) {
    return { ok: false, error: "notObject" };
  }
  const state: Walk = { skipped: [], imported: 0, maxDepth: limitDefault("document.jsonDepth") };
  const tokens = walk(parsed, [], "", state);
  return { ok: true, tokens, imported: state.imported, skipped: state.skipped };
}

export { flattenTokens };
