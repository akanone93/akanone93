// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useMemo, useRef, useState } from "react";
import { objects } from "../designers/db-schema-overlay.tsx";
import { hasErrors, type Json, useApply, useCheck, useDocumentValue } from "../designers/use-document.ts";
import { jsonEqual } from "../editor/json-patch.ts";
import { useDataContext } from "./scope.tsx";

/** The plugin document of the chosen plugin, with the unsaved edits over the server's. */
export function usePluginJson() {
  return useDocumentValue(useDataContext().documents, "plugin.json");
}

/** A list of the plugin document (`dataSources` or `collections`) as a local draft, seeded once the document has loaded. */
export function useSeededList(key: string): [Json[], (change: (all: Json[]) => Json[]) => void, boolean] {
  const { value } = usePluginJson();
  const [items, setItems] = useState<Json[]>([]);
  const seeded = useRef(false);
  useEffect(() => {
    if (value !== undefined && !seeded.current) {
      seeded.current = true;
      setItems(structuredClone(objects(value[key])));
    }
  }, [value, key]);
  return [items, (change) => setItems(change), seeded.current];
}

/** Validating and applying a draft of the plugin document through the edit pipeline (STU-063: only through it). */
export function useCommit(draft: Json | undefined, label: string) {
  const ctx = useDataContext();
  // The draft is rebuilt on every render; the validator and the confirmation follow its content, not its identity.
  const key = JSON.stringify(draft ?? null);
  // biome-ignore lint/correctness/useExhaustiveDependencies: `key` is the content of `draft`
  const next = useMemo(() => draft, [key]);
  const { path, value } = usePluginJson();
  const { canEdit, apply } = useApply(path, label);
  const check = useCheck(true, path, next, ctx.validatePage);
  const [applied, setApplied] = useState(false);
  // biome-ignore lint/correctness/useExhaustiveDependencies: a confirmation belongs to the draft it was made for
  useEffect(() => setApplied(false), [next]);
  const changed = next !== undefined && !jsonEqual(next, value);
  return {
    canEdit,
    check,
    changed,
    applied,
    canApply: canEdit && changed && !hasErrors(check),
    commit: () => {
      if (next !== undefined) {
        apply(next);
        setApplied(true);
      }
    },
  };
}
