// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Screen actions on a plugin's documents, written through the edit pipeline (STU-050, SRV-030). */

import type { EditSession, WholeWrite } from "../editor/edit-session.ts";
import type { JsonValue } from "../editor/json-patch.ts";
import {
  type Blocker,
  blankPage,
  deleteBlockers,
  duplicatePage,
  type IdSource,
  type PageDoc,
  slug,
  takenNames,
  uniqueName,
  uuidv7,
} from "./screens.ts";

interface DocLike {
  readonly path: string;
  readonly kind: string;
  readonly content: Uint8Array;
}

interface Parsed {
  readonly path: string;
  readonly kind: string;
  readonly json: PageDoc;
}

function parse(docs: readonly DocLike[]): Parsed[] {
  const decoder = new TextDecoder();
  return docs.flatMap((d) => {
    try {
      const json = JSON.parse(decoder.decode(d.content)) as PageDoc | null;
      return json !== null && typeof json === "object" ? [{ path: d.path, kind: d.kind, json }] : [];
    } catch {
      return [];
    }
  });
}

/** The plugin's documents (`plugin`) and every page of the app (`appPages`, for route names, SCH-025). */
export interface ScreenContext {
  readonly plugin: DocLike[];
  readonly app: readonly DocLike[];
}

export type ScreenResult =
  | WholeWrite
  | { readonly ok: false; readonly reason: "blocked"; readonly blockers: Blocker[] };

function context(ctx: ScreenContext) {
  const own = parse(ctx.plugin);
  const pluginDoc = own.find((d) => d.kind === "plugin");
  const ownPages = own.filter((d) => d.kind === "page");
  const appPages = parse(ctx.app).filter((d) => d.kind === "page");
  const names = takenNames(appPages.length > 0 ? appPages : ownPages);
  const prefix = pluginDoc === undefined ? "" : pluginDoc.path.slice(0, pluginDoc.path.length - "plugin.json".length);
  const entry = typeof pluginDoc?.json["entryPage"] === "string" ? pluginDoc.json["entryPage"] : undefined;
  return { pluginDoc, ownPages, names, prefix, entry, appPages: appPages.length > 0 ? appPages : ownPages };
}

function pageList(plugin: Parsed | undefined): string[] {
  const pages = plugin?.json["pages"];
  return Array.isArray(pages) ? pages.map(String) : [];
}

async function register(session: EditSession, pluginPath: string | undefined, id: string): Promise<void> {
  if (pluginPath !== undefined) {
    session.edit(pluginPath, (d) => {
      const pages = (d["pages"] as string[] | undefined) ?? [];
      d["pages"] = [...pages, id];
    });
  }
}

/** Creates a blank screen: the page document first, then its identifier in the plugin's page list. */
export async function createScreen(
  session: EditSession,
  ctx: ScreenContext,
  title: string,
  newId: IdSource = uuidv7,
): Promise<ScreenResult> {
  const c = context(ctx);
  const page = blankPage(title, c.names, newId);
  const written = await session.create(`${c.prefix}pages/${String(page["key"])}.page.json`, page as JsonValue);
  if (written.ok) {
    await register(session, c.pluginDoc?.path, String(page["id"]));
  }
  return written;
}

/** Duplicates a screen with fresh identifiers; references to things outside the page stay as they are. */
export async function duplicateScreen(
  session: EditSession,
  ctx: ScreenContext,
  pageId: string,
  copySuffix: string,
  newId: IdSource = uuidv7,
): Promise<ScreenResult> {
  const c = context(ctx);
  const source = c.ownPages.find((p) => p.json["id"] === pageId);
  if (source === undefined) {
    return { ok: false, reason: "unknown" };
  }
  const page = duplicatePage(source.json, c.names, copySuffix, newId);
  const written = await session.create(`${c.prefix}pages/${String(page["key"])}.page.json`, page as JsonValue);
  if (written.ok) {
    await register(session, c.pluginDoc?.path, String(page["id"]));
  }
  return written;
}

/**
 * Renames a screen. The title always changes. The route follows only when it still is the default
 * derived from the key and nothing navigates to it, so a rename never breaks a reference (SCH-025);
 * the key, which names the file, stays.
 */
export function renameScreen(session: EditSession, ctx: ScreenContext, pageId: string, title: string): ScreenResult {
  const c = context(ctx);
  const page = c.ownPages.find((p) => p.json["id"] === pageId);
  if (page === undefined) {
    return { ok: false, reason: "unknown" };
  }
  const referenced =
    deleteBlockers(
      page.json,
      undefined,
      c.appPages.map((p) => p.json),
    ).length > 0;
  const followsKey = page.json["route"] === page.json["key"];
  const newRoute =
    !referenced && followsKey
      ? uniqueName(slug(title), new Set([...c.names.routes].filter((r) => r !== page.json["route"])))
      : undefined;
  session.edit(page.path, (d) => {
    d["title"] = title;
    if (newRoute !== undefined) {
      d["route"] = newRoute;
    }
  });
  return { ok: true };
}

/** What stops a screen from being deleted; empty when it can go. */
export function blockersOf(ctx: ScreenContext, pageId: string): Blocker[] {
  const c = context(ctx);
  const page = c.ownPages.find((p) => p.json["id"] === pageId);
  return page === undefined
    ? []
    : deleteBlockers(
        page.json,
        c.entry,
        c.appPages.map((p) => p.json),
      );
}

/** Deletes a screen unless something blocks it: the page document, then its entry in the page list. */
export async function deleteScreen(session: EditSession, ctx: ScreenContext, pageId: string): Promise<ScreenResult> {
  const blockers = blockersOf(ctx, pageId);
  if (blockers.length > 0) {
    return { ok: false, reason: "blocked", blockers };
  }
  const c = context(ctx);
  const page = c.ownPages.find((p) => p.json["id"] === pageId);
  if (page === undefined) {
    return { ok: false, reason: "unknown" };
  }
  const removed = await session.remove(page.path);
  if (removed.ok && c.pluginDoc !== undefined && pageList(c.pluginDoc).includes(pageId)) {
    session.edit(c.pluginDoc.path, (d) => {
      d["pages"] = ((d["pages"] as string[] | undefined) ?? []).filter((id) => id !== pageId);
    });
  }
  return removed;
}
