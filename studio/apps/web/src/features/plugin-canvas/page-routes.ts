// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Dialog and sheet pages as the canvas draws them (PC-13, SCH-022): the page's root inside a design-time route node
 * the layout presents as the runtime does — over the page that opens it, under a scrim, on Material 3's dialog card or
 * modal sheet. Only the drawn copy is wrapped; the document holds the page as it is.
 */

import { ROUTE_TYPE, type RouteKind } from "@plux/layout";
import type { Node } from "./plugin-model.ts";

/** The prefix of the ids of the opener's widgets drawn under a dialog or sheet: no document node has them. */
export const UNDER = "under:";

/** A copy of `root` with every widget id prefixed, so the opener drawn underneath is never picked or edited. */
export function underlay(root: Node): Node {
  const copy = (node: Node): Node => {
    const out: Record<string, unknown> = { ...(node as unknown as Record<string, unknown>), id: `${UNDER}${node.id}` };
    if (node.children !== undefined) {
      out["children"] = node.children.map(copy);
    }
    if (node.slots !== undefined) {
      const slots: Record<string, unknown> = {};
      for (const [k, v] of Object.entries(node.slots as Readonly<Record<string, unknown>>)) {
        slots[k] = Array.isArray(v) ? v.map((x) => copy(x as Node)) : copy(v as Node);
      }
      out["slots"] = slots;
    }
    return out as unknown as Node;
  };
  return copy(root);
}

/** The route node that presents `page` as `kind`, over `below` (the opener, already an underlay) when there is one. */
export function routeRoot(kind: RouteKind, page: Node, below: Node | undefined): Node {
  return {
    id: `${page.id}~route`,
    type: ROUTE_TYPE,
    props: { kind } as never,
    slots: below === undefined ? { page } : { below, page },
  } as Node;
}

/** Whether a page of `kind` is presented on a route over another page. */
export const presented = (kind: string | undefined): kind is RouteKind => kind === "dialog" || kind === "bottomSheet";

/** Wraps dialog and sheet pages over their openers, keeping each wrapped root while its page and opener are unchanged. */
export class RouteCache {
  private readonly unders = new WeakMap<Node, Node>();
  private readonly routes = new WeakMap<Node, { below: Node | undefined; kind: RouteKind; node: Node }>();

  wrap(kind: RouteKind, page: Node, opener: Node | undefined): Node {
    let below: Node | undefined;
    if (opener !== undefined) {
      below = this.unders.get(opener);
      if (below === undefined) {
        below = underlay(opener);
        this.unders.set(opener, below);
      }
    }
    const kept = this.routes.get(page);
    if (kept !== undefined && kept.below === below && kept.kind === kind) {
      return kept.node;
    }
    const node = routeRoot(kind, page, below);
    this.routes.set(page, { below, kind, node });
    return node;
  }
}
