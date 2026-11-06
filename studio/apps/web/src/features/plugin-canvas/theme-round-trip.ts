// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The theme round trip on the canvas (PC-35, THM-001): the colours and sizes a widget sets — bound to a design token
 * or raw — what each token is and where on the screens it is used, a raw value turned into a token (the theme's own
 * with that value, else a new one), and token edits written to the theme, which the canvas draws as they are made.
 */

import type { PaintTheme, Rgba } from "@plux/layout";
import { putToken, setMode } from "../design/design-model.ts";
import { tokenFamilyOf } from "../panels/state-editors.tsx";
import { findNode } from "./select/edits.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

// Colour literals are document data; the "#" stays apart from the digits for the raw-value lint.
const HASH = "#";
const HEX = new RegExp(`^${HASH}([0-9a-fA-F]{6})([0-9a-fA-F]{2})?$`);

/** A `#RRGGBB` or `#RRGGBBAA` colour as `0xRRGGBBAA`; undefined for anything else. */
export function rgbaOf(v: unknown): Rgba | undefined {
  const m = typeof v === "string" ? HEX.exec(v) : null;
  return m === null ? undefined : Number.parseInt(`${m[1]}${m[2] ?? "ff"}`, 16) >>> 0;
}

/** `0xRRGGBBAA` as the document writes a colour, `#RRGGBBAA`. */
export const hexOf = (c: Rgba): string => `${HASH}${(c >>> 0).toString(16).padStart(8, "0").toUpperCase()}`;

/** One colour or size a widget sets, at `path` inside its props: bound to a token, or raw. */
export interface StyleValue {
  readonly path: readonly string[];
  readonly kind: "color" | "dimension";
  readonly token?: string;
  readonly raw?: Rgba | number;
  /** For a size, the token family it binds to first (`space`, `radius`, …). */
  readonly family?: string;
}

/** The family of a size at `path`: the nearest named key on the way that names one (`padding/all` is spacing). */
function familyAt(path: readonly string[]): string | undefined {
  for (const key of [...path].reverse()) {
    const f = tokenFamilyOf(key);
    if (f !== undefined) {
      return f;
    }
  }
  return undefined;
}

function valueAt(v: unknown, path: readonly string[]): StyleValue | undefined {
  if (isObj(v) && typeof v["$token"] === "string") {
    const token = v["$token"];
    const family = token.startsWith("color.") ? undefined : familyAt(path);
    return {
      path,
      kind: token.startsWith("color.") ? "color" : "dimension",
      token,
      ...(family === undefined ? {} : { family }),
    };
  }
  const colour = rgbaOf(v);
  if (colour !== undefined) {
    return { path, kind: "color", raw: colour };
  }
  const family = typeof v === "number" ? familyAt(path) : undefined;
  return family === undefined ? undefined : { path, kind: "dimension", raw: v as number, family };
}

/** The colours and sizes a widget's props set, bound or raw, down to three levels (a style's colour, a radius's corner). */
export function styleValuesOf(node: unknown): StyleValue[] {
  const out: StyleValue[] = [];
  const visit = (v: unknown, path: readonly string[]) => {
    const found = path.length === 0 ? undefined : valueAt(v, path);
    if (found !== undefined) {
      out.push(found);
      return;
    }
    if (isObj(v) && path.length < 3 && v["$expr"] === undefined) {
      for (const [k, child] of Object.entries(v)) {
        visit(child, [...path, k]);
      }
    }
  };
  visit(isObj(node) && isObj(node["props"]) ? node["props"] : {}, []);
  return out;
}

const kidsOf = (n: Obj): unknown[] => [
  ...(isObj(n["slots"]) ? Object.values(n["slots"]).flatMap((v) => (Array.isArray(v) ? v : [v])) : []),
  ...(Array.isArray(n["children"]) ? n["children"] : []),
];

/** Whether a value names the token anywhere inside it. */
function names(v: unknown, token: string): boolean {
  if (Array.isArray(v)) {
    return v.some((x) => names(x, token));
  }
  return isObj(v) && (v["$token"] === token || Object.values(v).some((x) => names(x, token)));
}

/** The widgets under `roots` whose props use `token`, in document order. */
export function tokenUsers(roots: readonly unknown[], token: string): string[] {
  const out: string[] = [];
  const visit = (n: unknown) => {
    if (!isObj(n)) {
      return;
    }
    if (typeof n["id"] === "string" && names(n["props"], token)) {
      out.push(n["id"]);
    }
    kidsOf(n).forEach(visit);
  };
  roots.forEach(visit);
  return out;
}

/**
 * The theme's token with exactly a raw value: a colour token of that colour; a size of that many pixels in the size's
 * own family (a radius binds to a radius token, never to a spacing one of the same pixels).
 */
export function matchingToken(v: StyleValue, theme: PaintTheme | null): string | undefined {
  if (v.raw === undefined || theme === null) {
    return undefined;
  }
  if (v.kind === "color") {
    return Object.entries(theme.tokens ?? {}).find(([, c]) => c === v.raw)?.[0];
  }
  return Object.entries(theme.dimensions ?? {}).find(
    ([path, px]) => px === v.raw && v.family !== undefined && path.startsWith(`${v.family}.`),
  )?.[0];
}

/** A name for a new token of a raw value that no token of the theme has: `space.18`, `color.custom2`. */
export function newTokenName(v: StyleValue, taken: ReadonlySet<string>): string {
  const base = v.kind === "color" ? "color.custom" : `${v.family ?? "size"}.${String(v.raw).replace(".", "_")}`;
  if (v.kind !== "color" && !taken.has(base)) {
    return base;
  }
  let n = 1;
  while (taken.has(`${base}${n}`)) {
    n += 1;
  }
  return `${base}${n}`;
}

/** The recipe that sets node `id`'s prop at `path` to `value`. */
export function setStyleRecipe(id: string, path: readonly string[], value: unknown): (root: unknown) => void {
  return (root) => {
    const node = findNode(root, id)?.node as Obj | undefined;
    if (node === undefined || path.length === 0) {
      return;
    }
    let at: Obj = isObj(node["props"]) ? node["props"] : {};
    node["props"] = at;
    for (const key of path.slice(0, -1)) {
      const next: Obj = isObj(at[key]) ? (at[key] as Obj) : {};
      at[key] = next;
      at = next;
    }
    at[path.at(-1) as string] = value;
  };
}

/** A token's document value: a colour as `#RRGGBBAA`, a size as pixels. */
const documentValue = (kind: StyleValue["kind"], value: Rgba | number) =>
  kind === "color" ? hexOf(value) : { unit: "px", value };

/** The theme recipe that adds a token of a raw value. */
export function addTokenRecipe(name: string, v: StyleValue): (tokens: Obj) => void {
  return (tokens) => {
    if (v.raw !== undefined) {
      putToken(
        tokens as never,
        name.split("."),
        documentValue(v.kind, v.raw),
        v.kind === "color" ? "color" : "dimension",
      );
    }
  };
}

/** The theme recipe that sets a token's value; in a dark preview, a colour's dark value when it has one. */
export function setTokenRecipe(
  name: string,
  kind: StyleValue["kind"],
  value: Rgba | number,
  dark: boolean,
): (tokens: Obj) => void {
  return (tokens) => {
    let leaf: unknown = tokens;
    for (const key of name.split(".")) {
      leaf = isObj(leaf) ? leaf[key] : undefined;
    }
    const ext = isObj(leaf) && isObj(leaf["$extensions"]) ? leaf["$extensions"] : {};
    const mode = kind === "color" && dark && ext["dev.plux.dark"] !== undefined ? "dark" : "light";
    setMode(tokens as never, name.split("."), mode, documentValue(kind, value));
  };
}
