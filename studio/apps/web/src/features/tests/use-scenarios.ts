// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Document } from "@plux/api";
import { useMemo } from "react";
import { useEditor } from "../editor/editor-provider.tsx";
import { isScenarioPath, parseBytes, type ScenarioRow, scenariosOf } from "./tests.model.ts";

/** The app's scenario files, the session's unsaved content over the server's, with the scenarios of each. */
export function useScenarioFiles(documents: readonly Document[] | undefined) {
  const local = useEditor((st) => st.docs);
  return useMemo(() => {
    const files = new Map<string, readonly ScenarioRow[]>();
    for (const d of documents ?? []) {
      if (isScenarioPath(d.path)) {
        files.set(d.path, scenariosOf(d.path, local[d.path]?.content ?? parseBytes(d.content)));
      }
    }
    for (const [path, doc] of Object.entries(local)) {
      if (isScenarioPath(path)) {
        files.set(path, scenariosOf(path, doc.content));
      }
    }
    return files;
  }, [documents, local]);
}
