// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { mergeInto } from "../json-editor/merge.ts";

/** What the list needs of a snapshot. */
export interface SnapshotRow {
  readonly id: string;
  readonly revision: bigint;
}

/** Two chosen snapshots as [older, newer] by revision; undefined until two are chosen. */
export function orderedPair(rows: readonly SnapshotRow[], chosen: readonly string[]): [string, string] | undefined {
  const picked = rows.filter((r) => chosen.includes(r.id));
  const [a, b] = picked;
  if (picked.length !== 2 || a === undefined || b === undefined) {
    return undefined;
  }
  return a.revision <= b.revision ? [a.id, b.id] : [b.id, a.id];
}

export interface SnapshotDocument {
  readonly path: string;
  readonly content: Uint8Array;
}

export interface RestoreEdit {
  readonly path: string;
  readonly recipe: (draft: Record<string, unknown>) => void;
}

/** One edit per snapshot document that parses as a JSON object, so restoring goes through the edit pipeline as one undo step. */
export function restoreRecipes(documents: readonly SnapshotDocument[]): RestoreEdit[] {
  return documents.flatMap((d) => {
    let parsed: unknown;
    try {
      parsed = JSON.parse(new TextDecoder().decode(d.content));
    } catch {
      return [];
    }
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      return [];
    }
    const next = parsed as Record<string, unknown>;
    return [{ path: d.path, recipe: (draft: Record<string, unknown>) => mergeInto(draft, next) }];
  });
}

/** The snapshot's documents that the open editor no longer has; the server brings these back. */
export function missingPaths(documents: readonly SnapshotDocument[], isOpen: (path: string) => boolean): string[] {
  return documents.filter((d) => !isOpen(d.path)).map((d) => d.path);
}

/** The short form of an editor-session id shown beside a snapshot. */
export function sessionTag(id: string): string {
  return id.slice(0, 8);
}
