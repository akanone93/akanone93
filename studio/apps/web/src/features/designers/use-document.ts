// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Problem } from "@plux/validator";
import { useEffect, useMemo, useRef, useState } from "react";
import { useCanEdit, useEditor, useEditSession } from "../editor/editor-provider.tsx";
import { mergeDeep } from "./merge-deep.ts";

export interface DesignerDocument {
  readonly path: string;
  readonly content: Uint8Array;
  /** The stored revision; the app's documents carry it so an overlay can open them in the edit session. */
  readonly revision?: bigint;
}

export type Json = Record<string, unknown>;

export const isObject = (v: unknown): v is Json => typeof v === "object" && v !== null && !Array.isArray(v);

/** Milliseconds after the last change before the validator worker checks a draft (the same as the JSON editor). */
export const DESIGNER_DEBOUNCE_MS = 300;

/** The document whose path is `name` or ends with `/name`, so a plugin's `plugin.json` and an app's `theme.json` are found wherever the layout puts them. */
export function findDocument(documents: readonly DesignerDocument[] | undefined, name: string) {
  return documents?.find((d) => d.path === name || d.path.endsWith(`/${name}`));
}

/** A document as the editor shows it: the session's unsaved content over the server's. */
export function useDocumentValue(documents: readonly DesignerDocument[] | undefined, name: string) {
  const local = useEditor((st) => st.docs);
  const doc = findDocument(documents, name);
  const path = doc?.path;
  return useMemo(() => {
    if (path === undefined) {
      return { path, value: undefined as Json | undefined };
    }
    const edited = local[path]?.content;
    if (isObject(edited)) {
      return { path, value: edited };
    }
    try {
      const parsed: unknown = JSON.parse(new TextDecoder().decode(doc?.content));
      return { path, value: isObject(parsed) ? parsed : undefined };
    } catch {
      return { path, value: undefined };
    }
  }, [local, path, doc]);
}

export type Check =
  | { readonly state: "checking" }
  | { readonly state: "unavailable" }
  | { readonly state: "done"; readonly problems: readonly Problem[] };

/** Checks the document with `next` merged in through the validator worker, `DESIGNER_DEBOUNCE_MS` after the last change. */
export function useCheck(
  open: boolean,
  path: string | undefined,
  next: Json | undefined,
  validatePage: ((file: string, data: string) => Promise<readonly Problem[]>) | undefined,
): Check {
  const [check, setCheck] = useState<Check>({ state: "checking" });
  useEffect(() => {
    if (!open || path === undefined || next === undefined) {
      return undefined;
    }
    if (validatePage === undefined) {
      setCheck({ state: "unavailable" });
      return undefined;
    }
    let live = true;
    setCheck({ state: "checking" });
    const timer = setTimeout(() => {
      validatePage(path, JSON.stringify(next))
        .then(
          (found) =>
            live && setCheck({ state: "done", problems: found.filter((p) => p.file === path || p.file === "") }),
        )
        .catch(() => live && setCheck({ state: "unavailable" }));
    }, DESIGNER_DEBOUNCE_MS);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [open, path, next, validatePage]);
  return check;
}

/** Applies `next` to the document through the edit pipeline as a minimal JSON Patch (STU-063: overlays write only through it). */
export function useApply(path: string | undefined, label: string) {
  const session = useEditSession();
  const canEdit = useCanEdit();
  const count = useRef(0);
  return {
    canEdit,
    apply: (next: Json) => {
      if (path === undefined || !canEdit) {
        return;
      }
      count.current += 1;
      session.edit(path, (draft) => mergeDeep(draft, next), { group: `${label}:${path}:${count.current}` });
    },
  };
}

export const hasErrors = (check: Check): boolean =>
  check.state !== "done" || check.problems.some((p) => p.severity === "error");
