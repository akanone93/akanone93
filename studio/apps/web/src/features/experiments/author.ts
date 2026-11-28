// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { DocumentService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";
import { lockTransport } from "../editor/lock-transport.ts";
import { uuidv7 } from "../plugin-canvas/screens.ts";
import { findNode } from "../plugin-canvas/select/edits.ts";
import { VARIANT_TYPE, variantOf } from "../plugin-canvas/variants.ts";

/**
 * What the compiler builds an experiment's variants from (ABT-001, STU-103, build log row 304): the project's
 * `experiments/<key>.experiment.json`, and for a subtree experiment the selected widgets wrapped in `Variant` nodes —
 * each the control unchanged and a copy per variant, to be changed on the canvas. Registering the experiment on the
 * server only allocates devices; without these a release shows every device the control.
 */

/** A JSON object with the fields this file reads named, so they read as properties. */
interface Obj {
  root?: unknown;
  key?: unknown;
  scope?: unknown;
  variants?: unknown;
  control?: unknown;
  name?: unknown;
  type?: unknown;
  slots?: unknown;
  [field: string]: unknown;
}
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The path of an experiment's document in the app's draft. */
export const experimentPath = (key: string) => `experiments/${key}.experiment.json`;

/** The variants a Variant node names: every variant of the document but its control. */
export function variantNames(doc: Obj): string[] {
  const variants = Array.isArray(doc.variants) ? doc.variants : [];
  return variants.flatMap((v) => (isObj(v) && v.control !== true && typeof v.name === "string" ? [v.name] : []));
}

/**
 * Wraps each node of `ids` found in page `page` in a Variant of experiment `key`, in place; returns how many it
 * wrapped. A page's root, a node already in a Variant arm's place and a node that is itself a Variant are left as they
 * are: a Variant replaces a widget among its parent's children or in its slot.
 */
export function wrapInVariants(
  page: unknown,
  ids: readonly string[],
  key: string,
  variants: readonly string[],
  fresh: () => string,
): number {
  const root = isObj(page) ? page.root : undefined;
  let wrapped = 0;
  for (const id of ids) {
    const p = findNode(root, id);
    if (p === null || p.parent === null || p.node.type === VARIANT_TYPE) {
      continue;
    }
    const variant = variantOf(p.node, key, variants, fresh);
    if (p.list !== null) {
      p.list[p.index] = variant;
    } else if (p.slot !== null && isObj(p.parent.slots)) {
      p.parent.slots[p.slot] = variant;
    } else {
      continue;
    }
    wrapped += 1;
  }
  return wrapped;
}

/**
 * Binds the prop `prop` of node `node` in page `page` to experiment `key`'s value of it
 * (`experiment.<key>.<prop>`, ABT-001, build log row 329), in place; returns whether the page has the node.
 */
export function bindProp(page: unknown, node: string, prop: string, key: string): boolean {
  const found = findNode(isObj(page) ? page.root : undefined, node);
  if (found === null) {
    return false;
  }
  const n = found.node as Obj;
  n["props"] = { ...(isObj(n["props"]) ? n["props"] : {}), [prop]: { $expr: `experiment.${key}.${prop}` } };
  return true;
}

/** The selection an experiment starts from: a plugin and the widgets selected on its canvas, or one prop of one. */
export interface ExperimentTarget {
  readonly pluginId: string;
  readonly ids: readonly string[];
  /** A prop experiment's widget and prop. */
  readonly prop?: { readonly node: string; readonly name: string };
}

/** Writes an experiment's document and, for a subtree experiment, its Variant nodes, under the editing locks. */
export function useAuthorExperiment(app: string) {
  const { session } = useApp();
  const documents = createClient(DocumentService, session.transport);
  const locked = async (pluginId: string, change: (lock: string) => Promise<unknown>) => {
    const lock = lockTransport(session.transport, { appId: app, pluginId });
    const editing = crypto.randomUUID();
    await lock.acquire(editing, false);
    try {
      await change(editing);
    } finally {
      await lock.release(editing, { keepalive: false }).catch(() => undefined);
    }
  };
  const pagesOf = async (pluginId: string) => {
    const out: { path: string; revision: bigint; json: unknown }[] = [];
    let pageToken = "";
    do {
      const r = await documents.listDocuments({ appId: app, pluginId, contentIncluded: true, page: { pageToken } });
      for (const d of r.documents.filter((x) => x.kind === "page")) {
        out.push({ path: d.path, revision: d.revision, json: JSON.parse(new TextDecoder().decode(d.content)) });
      }
      pageToken = r.page?.nextPageToken ?? "";
    } while (pageToken !== "");
    return out;
  };
  const bind = async (target: ExperimentTarget, key: string) => {
    const prop = target.prop;
    if (prop === undefined) {
      return;
    }
    await locked(target.pluginId, async (lock) => {
      for (const page of await pagesOf(target.pluginId)) {
        if (bindProp(page.json, prop.node, prop.name, key)) {
          await documents.putDocument({
            appId: app,
            pluginId: target.pluginId,
            session: lock,
            path: page.path,
            content: new TextEncoder().encode(JSON.stringify(page.json, null, 2)),
            ifRevision: page.revision,
          });
        }
      }
    });
  };
  return async (doc: Obj, target: ExperimentTarget | undefined): Promise<void> => {
    const key = String(doc.key);
    await locked("", (lock) =>
      documents.putDocument({
        appId: app,
        pluginId: "",
        session: lock,
        path: experimentPath(key),
        content: new TextEncoder().encode(JSON.stringify(doc, null, 2)),
        ifRevision: 0n,
      }),
    );
    if (target !== undefined && doc.scope === "prop") {
      await bind(target, key);
      await session.queryClient.invalidateQueries();
      return;
    }
    if (target === undefined || target.ids.length === 0 || doc.scope !== "subtree") {
      return;
    }
    const variants = variantNames(doc);
    await locked(target.pluginId, async (lock) => {
      for (const page of await pagesOf(target.pluginId)) {
        if (wrapInVariants(page.json, target.ids, key, variants, () => uuidv7()) > 0) {
          await documents.putDocument({
            appId: app,
            pluginId: target.pluginId,
            session: lock,
            path: page.path,
            content: new TextEncoder().encode(JSON.stringify(page.json, null, 2)),
            ifRevision: page.revision,
          });
        }
      }
    });
    await session.queryClient.invalidateQueries();
  };
}

/** Node `node`'s value of prop `prop` in page `page`, or `found: false` when the page has no such node. */
export function propIn(
  page: unknown,
  node: string,
  prop: string,
): { readonly found: boolean; readonly value?: unknown } {
  const found = findNode(isObj(page) ? page.root : undefined, node);
  if (found === null) {
    return { found: false };
  }
  const props = (found.node as Obj)["props"];
  return { found: true, value: isObj(props) ? props[prop] : undefined };
}

/** The value of prop `prop` of node `node` on the pages of plugin `pluginId`: what a prop experiment's control keeps. */
export function usePropValue(app: string, pluginId: string | undefined, node: string, prop: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["prop-value", app, pluginId, node, prop],
    enabled: pluginId !== undefined && node !== "",
    queryFn: async (): Promise<{ readonly value: unknown }> => {
      const documents = createClient(DocumentService, session.transport);
      let pageToken = "";
      do {
        const r = await documents.listDocuments({
          appId: app,
          pluginId: pluginId ?? "",
          contentIncluded: true,
          page: { pageToken },
        });
        const hit = r.documents
          .filter((d) => d.kind === "page")
          .map((d) => propIn(JSON.parse(new TextDecoder().decode(d.content)), node, prop))
          .find((x) => x.found);
        if (hit !== undefined) {
          return { value: hit.value };
        }
        pageToken = r.page?.nextPageToken ?? "";
      } while (pageToken !== "");
      return { value: undefined };
    },
  });
}
