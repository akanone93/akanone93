// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { limitDefault } from "@plux/schema";
import { DARK_KEY, flattenTokens, type TokenRow } from "../designers/theme-overlay.tsx";
import { isObject, type Json } from "../designers/use-document.ts";

/**
 * The reserved token groups of the theme (ADR-0032, `docs/reference/theming.md`), where the compiler and the device
 * read them: a token's high-contrast value is the token at `contrast.high.<path>` (THM-002), a brand's value the token
 * at `brands.<brand>.<path>` (THM-003), and the font families of a script the `fontFamily` token at
 * `font.script.<ISO 15924 code>` (THM-004).
 */
export const CONTRAST_GROUP = ["contrast", "high"] as const;
export const BRANDS_GROUP = ["brands"] as const;
export const SCRIPT_GROUP = ["font", "script"] as const;
export const ICONS_GROUP = ["icons"] as const;

export type Mode = "light" | "dark" | "contrast";

/** The scripts whose font families the theme defines with fallbacks (THM-004). */
export const SCRIPTS = ["latin", "cyrillic", "arabic", "devanagari", "ethiopic"] as const;

/** The ISO 15924 code of each script, the last segment of its `font.script` token. */
export const SCRIPT_CODES: Readonly<Record<(typeof SCRIPTS)[number], string>> = {
  latin: "Latn",
  cyrillic: "Cyrl",
  arabic: "Arab",
  devanagari: "Deva",
  ethiopic: "Ethi",
};

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

/** A DTCG alias: the path of another token in braces, such as `{color.primary}`. */
const ALIAS = /^\{[a-zA-Z0-9][a-zA-Z0-9-]*(\.[a-zA-Z0-9][a-zA-Z0-9-]*)*\}$/;

const RESERVED = [BRANDS_GROUP, CONTRAST_GROUP, SCRIPT_GROUP, ICONS_GROUP] as const;

const under = (path: readonly string[], prefix: readonly string[]): boolean =>
  path.length > prefix.length && prefix.every((p, i) => path[i] === p);

/** The base design tokens: everything outside the brand, high-contrast, script-font and icon groups. */
export const baseRows = (rows: readonly TokenRow[]): TokenRow[] =>
  rows.filter((r) => !RESERVED.some((group) => under(r.path, group)));

/** Rows below one reserved group, with the group's segments taken off their path. */
export const groupRows = (rows: readonly TokenRow[], group: (typeof RESERVED)[number]): TokenRow[] =>
  rows.filter((r) => under(r.path, group)).map((r) => ({ ...r, path: r.path.slice(group.length) }));

/** The names of the brand overlays: the segment after `brands`. */
export const brandNames = (rows: readonly TokenRow[]): string[] => [
  ...new Set(groupRows(rows, BRANDS_GROUP).flatMap((r) => (r.path[0] === undefined ? [] : [r.path[0]]))),
];

/** The font families of a `fontFamily` value: a list, or a single name. */
export const families = (value: unknown): string[] =>
  Array.isArray(value)
    ? value.filter((v): v is string => typeof v === "string")
    : typeof value === "string"
      ? [value]
      : [];

/** How many families (the first and its fallbacks) each script of THM-004 has. */
export function scriptCoverage(rows: readonly TokenRow[]): Record<(typeof SCRIPTS)[number], number> {
  const fonts = groupRows(rows, SCRIPT_GROUP);
  const count = (script: (typeof SCRIPTS)[number]) => fonts.find((r) => r.path[0] === SCRIPT_CODES[script])?.light;
  return Object.fromEntries(SCRIPTS.map((s) => [s, families(count(s)).length])) as Record<
    (typeof SCRIPTS)[number],
    number
  >;
}

/** The path of a script's `fontFamily` token. */
export const scriptPath = (script: (typeof SCRIPTS)[number]): string[] => [...SCRIPT_GROUP, SCRIPT_CODES[script]];

function leafAt(tokens: Json, path: readonly string[]): Json | undefined {
  let node: unknown = tokens;
  for (const key of path) {
    node = isObject(node) ? node[key] : undefined;
  }
  return isObject(node) ? node : undefined;
}

/** The `$type` of the token at `path`: its own, or the nearest group's above it (THM-001). */
export function typeAt(tokens: Json, path: readonly string[]): string | undefined {
  let node: unknown = tokens;
  let type: string | undefined;
  for (const key of path) {
    if (isObject(node) && typeof node["$type"] === "string") {
      type = node["$type"];
    }
    node = isObject(node) ? node[key] : undefined;
  }
  return isObject(node) && typeof node["$type"] === "string" ? node["$type"] : type;
}

/**
 * Sets or clears (`undefined`) the value of one mode of an existing token. Dark is the token's
 * `$extensions["dev.plux.dark"]`; high contrast is the token at `contrast.high.<path>`, typed as the token is, which
 * is where the compiler checks it (A11Y-003) and the device reads it.
 */
export function setMode(tokens: Json, path: readonly string[], mode: Mode, value: unknown): void {
  const leaf = leafAt(tokens, path);
  if (leaf === undefined) {
    return;
  }
  if (mode === "light") {
    leaf["$value"] = value;
    return;
  }
  if (mode === "contrast") {
    const at = [...CONTRAST_GROUP, ...path];
    if (value === undefined) {
      dropToken(tokens, at);
    } else {
      putToken(tokens, at, value, typeAt(tokens, path));
    }
    return;
  }
  const ext: Json = isObject(leaf["$extensions"]) ? leaf["$extensions"] : {};
  if (value === undefined) {
    delete ext[DARK_KEY];
  } else {
    ext[DARK_KEY] = value;
  }
  if (Object.keys(ext).length === 0) {
    delete leaf["$extensions"];
  } else {
    leaf["$extensions"] = ext;
  }
}

/** The high-contrast value of a token: the value at `contrast.high.<path>`. */
export function contrastOf(tokens: unknown, path: readonly string[]): unknown {
  return isObject(tokens) ? leafAt(tokens, [...CONTRAST_GROUP, ...path])?.["$value"] : undefined;
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
  const chain: Json[] = [tokens];
  for (const key of path.slice(0, -1)) {
    const next = chain.at(-1)?.[key];
    if (!isObject(next)) {
      return;
    }
    chain.push(next);
  }
  const last = path.at(-1);
  if (last === undefined) {
    return;
  }
  delete chain.at(-1)?.[last];
  for (let i = chain.length - 1; i > 0; i--) {
    const node = chain[i];
    const key = path[i - 1];
    if (node === undefined || key === undefined || Object.keys(node).some((k) => !k.startsWith("$"))) {
      return;
    }
    delete chain[i - 1]?.[key];
  }
}

/** Removes a base token with its high-contrast value and every brand's value for it, so nothing is left orphaned. */
export function dropBaseToken(tokens: Json, path: readonly string[]): void {
  dropToken(tokens, path);
  dropToken(tokens, [...CONTRAST_GROUP, ...path]);
  const brands = leafAt(tokens, BRANDS_GROUP);
  for (const brand of Object.keys(brands ?? {}).filter((k) => !k.startsWith("$"))) {
    dropToken(tokens, [...BRANDS_GROUP, brand, ...path]);
  }
}

/**
 * The value text typed for a new token, as its type stores it: a number for `number`, a numeric weight for
 * `fontWeight`, a list of families for a comma-separated `fontFamily`, JSON for a composite type's object; an alias
 * (`{color.primary}`) and anything else stays text.
 */
export function typedValue(type: string | undefined, text: string): unknown {
  const t = text.trim();
  if (ALIAS.test(t)) {
    return t;
  }
  if ((type === "number" || type === "fontWeight") && t !== "" && Number.isFinite(Number(t))) {
    return Number(t);
  }
  if (type === "fontFamily" && t.includes(",")) {
    return t
      .split(",")
      .map((f) => f.trim())
      .filter((f) => f !== "");
  }
  if ((t.startsWith("{") && t.endsWith("}")) || (t.startsWith("[") && t.endsWith("]"))) {
    try {
      return JSON.parse(t) as unknown;
    } catch {
      return t;
    }
  }
  return t;
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
  if (typeof value === "string" && ALIAS.test(value)) {
    return true;
  }
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
  const ext = node["$extensions"];
  const dark = isObject(ext) ? ext[DARK_KEY] : undefined;
  if (!validValue(type, node["$value"]) || (dark !== undefined && !validValue(type, dark))) {
    state.skipped.push({ path, reason: "value" });
    return {};
  }
  state.imported += 1;
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
