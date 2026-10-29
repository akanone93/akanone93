// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Node } from "../plugin-canvas/plugin-model.ts";
import { childNodes } from "../plugin-canvas/select/tree.ts";

/** A screen as inspect mode sees it: its ID and its root node. */
export interface InspectScreen {
  readonly id: string;
  readonly root: Node | undefined;
}

const prefix = "/pages/";

function walk(node: Node, nodeId: string, trail: readonly number[]): readonly number[] | undefined {
  if (node.id === nodeId) {
    return trail;
  }
  const kids = childNodes(node);
  for (let i = 0; i < kids.length; i++) {
    const kid = kids[i];
    const found = kid === undefined ? undefined : walk(kid, nodeId, [...trail, i]);
    if (found !== undefined) {
      return found;
    }
  }
  return undefined;
}

/**
 * The document path of a node, `/pages/<screen>/<child ordinals>` (DEV-030): the form the device reports
 * and accepts. Slots come before children, as in the compiler's source map.
 */
export function nodePathOf(screens: readonly InspectScreen[], nodeId: string): string | undefined {
  for (const screen of screens) {
    const trail = screen.root === undefined ? undefined : walk(screen.root, nodeId, []);
    if (trail !== undefined) {
      return `${prefix}${screen.id}${trail.map((i) => `/${i}`).join("")}`;
    }
  }
  return undefined;
}

/** The screen and node ID a device-reported path names, or undefined when it names nothing here. */
export function resolveNodePath(
  screens: readonly InspectScreen[],
  path: string,
): { readonly screen: string; readonly node: string } | undefined {
  if (!path.startsWith(prefix)) {
    return undefined;
  }
  const [screenId, ...ordinals] = path.slice(prefix.length).split("/");
  let node = screens.find((s) => s.id === screenId)?.root;
  for (const ordinal of ordinals) {
    node = node === undefined ? undefined : childNodes(node)[Number(ordinal)];
  }
  return node === undefined || screenId === undefined ? undefined : { screen: screenId, node: node.id };
}
