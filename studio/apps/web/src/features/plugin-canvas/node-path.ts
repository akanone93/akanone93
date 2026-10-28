// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Node } from "./plugin-model.ts";
import { childNodes } from "./select/tree.ts";

/** The screen's root plus the child ordinals below it: the `root/0/2` form a runtime error reports (ANL-001). */
const pathPattern = /^root((?:\/\d+)*)$/;

/**
 * The document node ID a runtime `nodePath` names in `root`, or undefined when the path is not of the
 * `root/<ordinal>…` form or leaves the tree. Ordinals count slots first, then children, as the compiler's
 * source map does (`childNodes`).
 */
export function nodeIdAtPath(root: Node | undefined, path: string): string | undefined {
  const match = pathPattern.exec(path);
  if (root === undefined || match === null) {
    return undefined;
  }
  let node: Node | undefined = root;
  for (const ordinal of (match[1] ?? "").split("/").filter((s) => s !== "")) {
    node = node === undefined ? undefined : childNodes(node)[Number(ordinal)];
  }
  return node?.id;
}

/**
 * What to select for the canvas `node` parameter: the value itself when it is a node ID of the loaded screens,
 * else the node a `root/0` path names in the focused screen, else nothing (the page opens without a selection).
 */
export function resolveNodeParam(
  node: string,
  ids: ReadonlySet<string>,
  focused: { readonly root: Node | undefined } | undefined,
): string | undefined {
  if (ids.has(node)) {
    return node;
  }
  const mapped = nodeIdAtPath(focused?.root, node);
  return mapped !== undefined && ids.has(mapped) ? mapped : undefined;
}
