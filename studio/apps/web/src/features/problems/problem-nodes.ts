// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Problem } from "@plux/validator";

/**
 * Problems pinned to the widgets they are about (PC-16, PC-34): a problem's JSON Pointer is followed into its page
 * document, and the deepest widget node on the way (an object with an `id` and a `type`) owns it; in a page's timelines,
 * the widget a track animates or a driver follows does.
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

/** The widget a value is about: a widget node itself, or in a timeline a track or driver naming its widget. */
function ownerOf(at: unknown, timeline: boolean): string | undefined {
  if (!isObj(at) || Array.isArray(at)) {
    return undefined;
  }
  if (typeof at["id"] === "string" && typeof at["type"] === "string") {
    return at["id"];
  }
  return timeline && typeof at["node"] === "string" ? at["node"] : undefined;
}

/** The id of the deepest widget node `pointer` passes through in `doc`, or the widget a timeline's part names. */
export function nodeOfPointer(doc: unknown, pointer: string): string | undefined {
  let at: unknown = doc;
  let node: string | undefined;
  const path = segments(pointer);
  // A page's timelines name the widgets their tracks animate and their drivers follow (PC-34, ANI-008).
  const timeline = path[0] === "animations";
  for (const seg of path) {
    if (!isObj(at)) {
      break;
    }
    at = Array.isArray(at) ? at[Number(seg)] : at[seg];
    node = ownerOf(at, timeline) ?? node;
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
