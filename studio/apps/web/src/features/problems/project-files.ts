// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Problem, ProjectFile } from "@plux/validator";

interface DocumentLike {
  readonly path: string;
  readonly content: Uint8Array;
}

const decoder = new TextDecoder();

/** The documents as project files, text decoded; the validator loads them once on open. */
export function projectFiles(documents: readonly DocumentLike[]): ProjectFile[] {
  return documents.map((d) => ({ path: d.path, data: decoder.decode(d.content) }));
}

/** Problem counts by severity. */
export function countProblems(problems: readonly Problem[]): Record<Problem["severity"], number> {
  const n = { error: 0, warning: 0, info: 0 };
  for (const p of problems) {
    n[p.severity] += 1;
  }
  return n;
}

/** Problems grouped by file, files in first-seen order, project-level problems (no file) first. */
export function groupByFile(problems: readonly Problem[]): [string, Problem[]][] {
  const groups = new Map<string, Problem[]>();
  for (const p of problems) {
    const list = groups.get(p.file);
    if (list === undefined) {
      groups.set(p.file, [p]);
    } else {
      list.push(p);
    }
  }
  return [...groups].sort(([a], [b]) => (a === "" ? -1 : b === "" ? 1 : 0));
}
