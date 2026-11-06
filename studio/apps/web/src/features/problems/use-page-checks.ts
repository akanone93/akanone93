// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Every page checked in full (PC-34, SCH-040): loading the project checks each document's structure; a page's
 * semantic and policy checks — references, types, PXL, accessibility, budgets, expensive animations (ANI-008) — run
 * one page at a time, so each page is validated once the project is loaded and again, shortly after, whenever it
 * changes (a page whose document is unchanged keeps its findings). A page's own findings replace what the load found
 * in its file, so the counts, the Problems panel and the problems pinned to widgets all show them.
 */

import type { Problem } from "@plux/validator";
import { useEffect, useMemo, useRef, useState } from "react";
import type { Validation } from "./use-validation.ts";

/** How long the checks wait after an edit, so a burst of edits is checked once. */
export const PAGE_CHECK_DELAY_MS = 300;

interface PageDoc {
  readonly path: string;
  readonly kind: string;
  readonly parsed: unknown;
}

const keyOf = (p: Problem) => `${p.file}\u0000${p.path}\u0000${p.code}\u0000${p.message}`;

/** The load's problems with each checked page's findings in place of its file's; each problem once. */
export function mergedProblems(
  loaded: readonly Problem[],
  pages: ReadonlyMap<string, readonly Problem[]>,
): readonly Problem[] {
  const seen = new Set<string>();
  return [...loaded.filter((p) => !pages.has(p.file)), ...[...pages.values()].flat()].filter((p) => {
    const k = keyOf(p);
    if (seen.has(k)) {
      return false;
    }
    seen.add(k);
    return true;
  });
}

export function usePageChecks(validation: Validation, documents: readonly PageDoc[]): Validation {
  const done = validation.state === "done" ? validation : undefined;
  const [found, setFound] = useState<ReadonlyMap<string, readonly Problem[]>>(new Map());
  // Findings by document object, for the validator they came from: an unchanged page is not checked again.
  const cache = useRef<{ validator: unknown; byDoc: WeakMap<object, readonly Problem[]> }>({
    validator: undefined,
    byDoc: new WeakMap(),
  });
  useEffect(() => {
    if (done === undefined) {
      return undefined;
    }
    if (cache.current.validator !== done.validatePage) {
      cache.current = { validator: done.validatePage, byDoc: new WeakMap() };
    }
    const { byDoc } = cache.current;
    const pages = documents.filter(
      (d): d is PageDoc & { parsed: object } => d.kind === "page" && typeof d.parsed === "object" && d.parsed !== null,
    );
    let live = true;
    const timer = setTimeout(() => {
      void Promise.all(
        pages.map(async (d) => {
          const known = byDoc.get(d.parsed);
          const problems = known ?? (await done.validatePage(d.path, JSON.stringify(d.parsed)));
          byDoc.set(d.parsed, problems);
          return [d.path, problems] as const;
        }),
      )
        .then((entries) => {
          if (live) {
            setFound(new Map(entries));
          }
        })
        .catch(() => undefined);
    }, PAGE_CHECK_DELAY_MS);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [done, documents]);
  return useMemo(
    () =>
      done === undefined || found.size === 0 ? validation : { ...done, problems: mergedProblems(done.problems, found) },
    [validation, done, found],
  );
}
