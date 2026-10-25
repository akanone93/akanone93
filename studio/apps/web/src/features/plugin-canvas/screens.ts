// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Screen documents: a blank page, a duplicate with fresh identifiers, and what blocks a delete (STU-050, SCH-002, SCH-025). */

import type { JsonValue } from "../editor/json-patch.ts";

export type IdSource = () => string;

/** A UUIDv7 in canonical lower-case form (SCH-002): 48 bits of milliseconds, then random bits. */
export function uuidv7(now: number = Date.now(), random: (n: number) => Uint8Array = randomBytes): string {
  const b = random(16);
  const ms = BigInt(now);
  for (let i = 0; i < 6; i += 1) {
    b[i] = Number((ms >> BigInt(8 * (5 - i))) & 0xffn);
  }
  b[6] = ((b[6] ?? 0) & 0x0f) | 0x70;
  b[8] = ((b[8] ?? 0) & 0x3f) | 0x80;
  const h = Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}

function randomBytes(n: number): Uint8Array {
  return globalThis.crypto.getRandomValues(new Uint8Array(n));
}

export interface PageDoc {
  readonly [key: string]: JsonValue | undefined;
}

export interface ScreenNames {
  readonly keys: ReadonlySet<string>;
  readonly routes: ReadonlySet<string>;
}

/** `base`, else `base-2`, `base-3`, … the first not in `taken`. */
export function uniqueName(base: string, taken: ReadonlySet<string>): string {
  if (!taken.has(base)) {
    return base;
  }
  let n = 2;
  while (taken.has(`${base}-${n}`)) {
    n += 1;
  }
  return `${base}-${n}`;
}

/** Lower-kebab slug of a title, `screen` when nothing usable remains. */
export function slug(title: string): string {
  const s = title
    .normalize("NFKD")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return s === "" ? "screen" : s;
}

/** The names already used by the page documents (keys per plugin, routes app-wide, SCH-025). */
export function takenNames(pages: readonly { readonly json: unknown }[]): ScreenNames {
  const keys = new Set<string>();
  const routes = new Set<string>();
  for (const { json } of pages) {
    const p = json as { key?: unknown; route?: unknown } | null;
    if (typeof p?.key === "string") {
      keys.add(p.key);
    }
    if (typeof p?.route === "string") {
      routes.add(p.route);
    }
  }
  return { keys, routes };
}

/** A valid empty page (SCH-022): a `Scaffold` root, key and route unique among the names given. */
export function blankPage(title: string, names: ScreenNames, newId: IdSource = uuidv7): PageDoc {
  const key = uniqueName(slug(title), names.keys);
  return {
    schemaVersion: "1.0.0",
    kind: "page",
    id: newId(),
    key,
    route: uniqueName(key, names.routes),
    pageKind: "screen",
    title,
    root: { id: newId(), type: "Scaffold" },
  };
}

function collectNodeIds(value: JsonValue | undefined, out: Set<string>): void {
  if (Array.isArray(value)) {
    for (const v of value) {
      collectNodeIds(v, out);
    }
  } else if (value !== null && typeof value === "object") {
    const o = value as { [key: string]: JsonValue | undefined };
    if (
      typeof o["id"] === "string" &&
      /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(o["id"])
    ) {
      out.add(o["id"]);
    }
    for (const v of Object.values(o)) {
      collectNodeIds(v, out);
    }
  }
}

function remap(value: JsonValue, map: ReadonlyMap<string, string>): JsonValue {
  if (typeof value === "string") {
    return map.get(value) ?? value;
  }
  if (Array.isArray(value)) {
    return value.map((v) => remap(v, map));
  }
  if (value !== null && typeof value === "object") {
    const out: { [key: string]: JsonValue } = {};
    for (const [k, v] of Object.entries(value as { [key: string]: JsonValue })) {
      out[map.get(k) ?? k] = remap(v, map);
    }
    return out;
  }
  return value;
}

/**
 * Gives every identifier defined inside `doc` (the page's own and each node's) a fresh one and
 * rewrites every reference to them inside the document, in values and in keys. References to
 * identifiers defined elsewhere (translations, components, other pages) stay as they are.
 */
export function remapIds(doc: PageDoc, newId: IdSource = uuidv7): PageDoc {
  const ids = new Set<string>();
  collectNodeIds(doc as JsonValue, ids);
  const map = new Map<string, string>();
  for (const id of [...ids].sort()) {
    map.set(id, newId());
  }
  return remap(doc as JsonValue, map) as PageDoc;
}

/** A copy of a page: fresh identifiers, key and route made unique, the title marked as a copy when it is literal. */
export function duplicatePage(
  source: PageDoc,
  names: ScreenNames,
  copySuffix: string,
  newId: IdSource = uuidv7,
): PageDoc {
  const copy = remapIds(source, newId) as { [key: string]: JsonValue | undefined };
  const key = uniqueName(`${String(source["key"])}-copy`, names.keys);
  copy["key"] = key;
  if (typeof source["route"] === "string") {
    copy["route"] = uniqueName(`${source["route"]}-copy`, names.routes);
  }
  if (typeof source["title"] === "string") {
    copy["title"] = `${source["title"]} ${copySuffix}`;
  }
  return copy as PageDoc;
}

export interface Blocker {
  readonly kind: "entry" | "navigate";
  /** The page holding a navigate step to this one; absent for the entry page. */
  readonly pageId?: string;
  readonly title?: string;
}

function navigatesTo(value: unknown, route: string): boolean {
  if (Array.isArray(value)) {
    return value.some((v) => navigatesTo(v, route));
  }
  if (value !== null && typeof value === "object") {
    const o = value as Record<string, unknown>;
    if (o["action"] === "navigate" && (o["input"] as { route?: unknown } | undefined)?.route === route) {
      return true;
    }
    return Object.values(o).some((v) => navigatesTo(v, route));
  }
  return false;
}

/**
 * What stops a page from being deleted: it is the plugin's entry page (`entryPage` must name a
 * page, SCH-022) or another page of the app navigates to its route (the compiler rejects a
 * dangling route, ACT-040). The delete is refused while any blocker remains.
 */
export function deleteBlockers(page: PageDoc, entryPage: string | undefined, allPages: readonly PageDoc[]): Blocker[] {
  const out: Blocker[] = [];
  if (entryPage !== undefined && page["id"] === entryPage) {
    out.push({ kind: "entry" });
  }
  const route = typeof page["route"] === "string" ? page["route"] : undefined;
  if (route !== undefined) {
    for (const other of allPages) {
      if (other["id"] !== page["id"] && navigatesTo(other["root"], route)) {
        out.push({ kind: "navigate", pageId: String(other["id"]), title: String(other["title"] ?? other["key"]) });
      }
    }
  }
  return out;
}
