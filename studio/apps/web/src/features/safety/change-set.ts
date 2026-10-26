// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Atomic change sets across the documents and plugins of an app (P11 contracts #15): the assisted
 * fixes of a blocked delete (STU-070), moving a screen to another plugin (D23, STU-055) and undoing
 * either as one step across plugins (F3). Each is built from the loaded documents as JSON Patch
 * with `test` operations guarding what it read, and written by DocumentService.ApplyChangeSet in
 * one transaction, so a change set lands whole or not at all.
 */

import { createClient, type Transport } from "@connectrpc/connect";
import { DocumentService } from "@plux/api";
import type { JsonPatchOp, JsonValue } from "../editor/json-patch.ts";
import type { SessionEntry } from "../editor/undo-stack.ts";
import type { LoadedDoc, PageInfo, Use, WhereUsedIndex } from "./where-used.ts";

/** One document a change set writes: exactly one of a patch, whole content, or a deletion. */
export type ChangeSetEdit =
  | { readonly path: string; readonly ifRevision: number; readonly patch: readonly JsonPatchOp[] }
  | { readonly path: string; readonly ifRevision: 0; readonly content: JsonValue }
  | { readonly path: string; readonly ifRevision: number; readonly delete: true };

/** The part of a change set that falls on one draft; `pluginId` is "" for the app's. */
export interface ChangeSetDraft {
  readonly pluginId: string;
  readonly edits: readonly ChangeSetEdit[];
}

export type ChangeSet = readonly ChangeSetDraft[];

/** Groups patch operations by document and draft, keeping each document's operations in order. */
function group(
  docs: readonly LoadedDoc[],
  ops: readonly { readonly path: string; readonly op: JsonPatchOp }[],
): ChangeSet {
  const byPath = new Map<string, JsonPatchOp[]>();
  for (const { path, op } of ops) {
    byPath.set(path, [...(byPath.get(path) ?? []), op]);
  }
  const drafts = new Map<string, ChangeSetEdit[]>();
  for (const [path, patch] of byPath) {
    const doc = docs.find((d) => d.path === path);
    if (doc === undefined) {
      continue;
    }
    drafts.set(doc.pluginId, [...(drafts.get(doc.pluginId) ?? []), { path, ifRevision: doc.revision, patch }]);
  }
  return [...drafts].map(([pluginId, edits]) => ({ pluginId, edits }));
}

/** "Redirect all incoming links to…": every link into a screen now goes to another route. */
export function redirectIncoming(
  docs: readonly LoadedDoc[],
  incoming: readonly Use[],
  from: PageInfo,
  toRoute: string,
): ChangeSet {
  const links = incoming.filter((u) => u.kind === "navigates");
  return group(
    docs,
    links.flatMap((u) => [
      { path: u.path, op: { op: "test", path: u.pointer, value: from.route } as const },
      { path: u.path, op: { op: "replace", path: u.pointer, value: toRoute } as const },
    ]),
  );
}

/** Sorts JSON pointers so that removing array items one by one never shifts a later one. */
function laterFirst(a: string, b: string): number {
  const pa = a.split("/");
  const pb = b.split("/");
  for (let i = 0; i < Math.min(pa.length, pb.length); i++) {
    const x = pa[i] ?? "";
    const y = pb[i] ?? "";
    if (x !== y) {
      const nx = Number(x);
      const ny = Number(y);
      return Number.isInteger(nx) && Number.isInteger(ny) ? ny - nx : y.localeCompare(x);
    }
  }
  return pb.length - pa.length;
}

/** "Remove outgoing links": every navigation step leaving a plugin is removed. */
export function removeOutgoing(docs: readonly LoadedDoc[], outgoing: readonly Use[]): ChangeSet {
  const steps = outgoing.filter((u): u is Use & { readonly step: string } => u.step !== undefined);
  const tests = steps.map((u) => ({
    path: u.path,
    op: { op: "test", path: u.pointer, value: routeAt(docs, u) } as const,
  }));
  const removals = [...steps]
    .sort((a, b) => laterFirst(a.step, b.step))
    .map((u) => ({ path: u.path, op: { op: "remove", path: u.step } as const }));
  return group(docs, [...tests, ...removals]);
}

function routeAt(docs: readonly LoadedDoc[], use: Use): string {
  let v: unknown = docs.find((d) => d.path === use.path)?.json;
  for (const token of use.pointer.split("/").slice(1)) {
    const key = token.replaceAll("~1", "/").replaceAll("~0", "~");
    v = v !== null && typeof v === "object" ? (v as Record<string, unknown>)[key] : undefined;
  }
  return typeof v === "string" ? v : "";
}

export type MoveResult =
  | { readonly ok: true; readonly changeSet: ChangeSet }
  | {
      readonly ok: false;
      readonly reason: "unknown" | "same-plugin" | "entry" | "key-taken" | "private-component";
    };

/**
 * Moves a screen to another plugin with its links (D23, STU-055): the page leaves the source's
 * page list and directory and joins the target's, keeping its ID, key and route. Links are by
 * route, which stays, so every link into and out of the screen keeps working. The plugin's
 * entry screen cannot move until another is chosen; a screen using a component private to its
 * plugin cannot move to another.
 */
export function moveScreen(
  docs: readonly LoadedDoc[],
  index: WhereUsedIndex,
  pageId: string,
  targetPluginId: string,
): MoveResult {
  const page = index.pages.get(pageId);
  const source = docs.find((d) => d.path === page?.path);
  const sourcePlugin = docs.find((d) => d.kind === "plugin" && d.pluginId === page?.pluginId);
  const targetPlugin = docs.find((d) => d.kind === "plugin" && d.pluginId === targetPluginId);
  if (page === undefined || source === undefined || sourcePlugin === undefined || targetPlugin === undefined) {
    return { ok: false, reason: "unknown" };
  }
  if (page.pluginId === targetPluginId) {
    return { ok: false, reason: "same-plugin" };
  }
  if (index.usesOf("page", pageId).some((u) => u.kind === "entryPage")) {
    return { ok: false, reason: "entry" };
  }
  const targetDir = targetPlugin.path.slice(0, targetPlugin.path.length - "plugin.json".length);
  const targetPath = `${targetDir}pages/${page.key}.page.json`;
  if (docs.some((d) => d.path === targetPath)) {
    return { ok: false, reason: "key-taken" };
  }
  const privateUse = index.uses.some((u) => {
    if (u.path !== page.path || u.kind !== "usesComponent") {
      return false;
    }
    const component = docs.find((d) => d.kind === "component" && idOf(d.json) === u.target.id);
    const exported = component !== undefined && (component.json as Record<string, unknown>)["exported"] === true;
    return component !== undefined && component.pluginId === page.pluginId && !exported;
  });
  if (privateUse) {
    return { ok: false, reason: "private-component" };
  }
  const sourcePages = pagesOf(sourcePlugin);
  const at = sourcePages.indexOf(pageId);
  const sourceEdits: ChangeSetEdit[] = [{ path: source.path, ifRevision: source.revision, delete: true }];
  if (at >= 0) {
    sourceEdits.push({
      path: sourcePlugin.path,
      ifRevision: sourcePlugin.revision,
      patch: [
        { op: "test", path: `/pages/${at}`, value: pageId },
        { op: "remove", path: `/pages/${at}` },
      ],
    });
  }
  const targetPages = pagesOf(targetPlugin);
  return {
    ok: true,
    changeSet: [
      { pluginId: page.pluginId, edits: sourceEdits },
      {
        pluginId: targetPluginId,
        edits: [
          { path: targetPath, ifRevision: 0, content: source.json as JsonValue },
          {
            path: targetPlugin.path,
            ifRevision: targetPlugin.revision,
            patch:
              targetPlugin.json !== null && typeof targetPlugin.json === "object" && "pages" in targetPlugin.json
                ? [
                    { op: "test", path: "/pages", value: targetPages },
                    { op: "add", path: "/pages/-", value: pageId },
                  ]
                : [{ op: "add", path: "/pages", value: [pageId] }],
          },
        ],
      },
    ],
  };
}

function idOf(json: unknown): string | undefined {
  const id = json !== null && typeof json === "object" ? (json as Record<string, unknown>)["id"] : undefined;
  return typeof id === "string" ? id : undefined;
}

function pagesOf(plugin: LoadedDoc): string[] {
  const pages =
    plugin.json !== null && typeof plugin.json === "object"
      ? (plugin.json as Record<string, unknown>)["pages"]
      : undefined;
  return Array.isArray(pages) ? pages.map(String) : [];
}

/** A session entry with the draft it belongs to, as listed per plugin. */
export interface DraftEntry extends SessionEntry {
  readonly pluginId: string;
}

/**
 * Undoes one step across plugins (F3): the inverses the server stored for the step become a change
 * set. A created document's inverse removes the root and deletes it; a deleted document's adds the
 * root and creates it again; any other is a patch on the current revision. `revisions` holds the
 * current revision of every loaded document.
 */
export function undoStep(
  entries: readonly DraftEntry[],
  step: string,
  revisions: ReadonlyMap<string, number>,
): ChangeSet {
  const drafts = new Map<string, ChangeSetEdit[]>();
  for (const e of entries) {
    if (e.step !== step) {
      continue;
    }
    const [first] = e.inverse;
    const revision = revisions.get(e.path) ?? 0;
    let edit: ChangeSetEdit;
    if (e.inverse.length === 1 && first?.path === "" && first.op === "remove") {
      edit = { path: e.path, ifRevision: revision, delete: true };
    } else if (e.inverse.length === 1 && first?.path === "" && first.op === "add" && first.value !== undefined) {
      edit = { path: e.path, ifRevision: 0, content: first.value };
    } else {
      edit = { path: e.path, ifRevision: revision, patch: e.inverse };
    }
    drafts.set(e.pluginId, [...(drafts.get(e.pluginId) ?? []), edit]);
  }
  return [...drafts].map(([pluginId, edits]) => ({ pluginId, edits }));
}

const encoder = new TextEncoder();

/** Sends a change set; `sessionOf` names the lock session that holds each draft's lock. */
export async function applyChangeSet(
  transport: Transport,
  ids: { readonly appId: string; readonly editorSessionId: string; readonly stepId: string },
  changeSet: ChangeSet,
  sessionOf: (pluginId: string) => Promise<string>,
): Promise<void> {
  const client = createClient(DocumentService, transport);
  const drafts = await Promise.all(
    changeSet.map(async (d) => ({
      pluginId: d.pluginId,
      session: await sessionOf(d.pluginId),
      edits: d.edits.map((e) => ({
        path: e.path,
        ifRevision: BigInt(e.ifRevision),
        ...("patch" in e
          ? {
              patch: e.patch.map((o) => ({
                op: o.op,
                path: o.path,
                value: o.value === undefined ? new Uint8Array() : encoder.encode(JSON.stringify(o.value)),
              })),
            }
          : {}),
        ...("content" in e ? { content: encoder.encode(JSON.stringify(e.content)) } : {}),
        ...("delete" in e ? { delete: true } : {}),
      })),
    })),
  );
  await client.applyChangeSet({ ...ids, drafts });
}
