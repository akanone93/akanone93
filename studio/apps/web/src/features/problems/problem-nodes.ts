// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Problem } from "@plux/validator";

/**
 * Problems pinned to the widgets they are about (PC-16): a problem's JSON Pointer is followed into its page document,
 * and the deepest widget node on the way (an object with an `id` and a `type`) owns it.
 */

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null;

/** A JSON Pointer's segments, unescaped (RFC 6901). */
function segments(pointer: string): string[] {
  return pointer === ""
    ? []
    : pointer
        .slice(1)
        .split("/")
        .map((s) => s.replace(/~1/g, "/").replace(/~0/g, "~"));
}

/** The id of the deepest widget node `pointer` passes through in `doc`, if any. */
export function nodeOfPointer(doc: unknown, pointer: string): string | undefined {
  let at: unknown = doc;
  let node: string | undefined;
  for (const seg of segments(pointer)) {
    if (!isObj(at)) {
      break;
    }
    at = Array.isArray(at) ? at[Number(seg)] : at[seg];
    if (isObj(at) && !Array.isArray(at) && typeof at["id"] === "string" && typeof at["type"] === "string") {
      node = at["id"];
    }
  }
  return node;
}

export interface NodeProblem {
  readonly severity: Problem["severity"];
  readonly message: string;
  readonly code: string;
}

const rank: Readonly<Record<Problem["severity"], number>> = { error: 0, warning: 1, info: 2 };

/** The problems of each widget, the most severe first; `docs` are the page documents by path, parsed. */
export function problemsByNode(
  problems: readonly Problem[],
  docs: ReadonlyMap<string, unknown>,
): ReadonlyMap<string, readonly NodeProblem[]> {
  const out = new Map<string, NodeProblem[]>();
  for (const p of problems) {
    const doc = docs.get(p.file);
    const id = doc === undefined ? undefined : nodeOfPointer(doc, p.path);
    if (id !== undefined) {
      const list = out.get(id) ?? [];
      list.push({ severity: p.severity, message: p.message, code: p.code });
      out.set(id, list);
    }
  }
  for (const list of out.values()) {
    list.sort((a, b) => rank[a.severity] - rank[b.severity]);
  }
  return out;
}
