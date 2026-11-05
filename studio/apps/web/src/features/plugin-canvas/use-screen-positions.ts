// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useCallback, useMemo, useRef, useState } from "react";
import { useEditor, useEditSession } from "../editor/editor-provider.tsx";
import { canvasPathOf, type Point, positionsDocument, positionsOf, positionsRecipe } from "./positions.ts";
import { uuidv7 } from "./screens.ts";

export interface ScreenPositions {
  /** The positions the canvas shows: a drop on its way to the edit session, else the session's own copy. */
  readonly positions: ReadonlyMap<string, Point> | undefined;
  /** Saves `positions` (every screen's place) as one undoable change; false while the plugin cannot be edited. */
  readonly commit: (positions: ReadonlyMap<string, Point>) => boolean;
  /** Whether the last creation of the document failed (offline or refused); the page says so. */
  readonly failed: boolean;
}

/**
 * Where the plugin's screens stand (STU-050): the editor-only `canvas` document beside `plugin.json`, created by the
 * first drop and edited by every later one. What is shown is the edit session's copy, pending edits included, so the
 * server confirming an earlier save or a refetch cannot move a screen back while a later drop is on its way; a drop
 * runs ahead of it only until the session holds it. A drop made while the creation is in flight is applied once it
 * lands.
 */
export function useScreenPositions(
  documents: readonly { readonly kind: string; readonly path: string }[] | undefined,
  canEdit: boolean,
): ScreenPositions {
  const session = useEditSession();
  const pluginPath = documents?.find((d) => d.kind === "plugin")?.path;
  const path = pluginPath === undefined ? undefined : canvasPathOf(pluginPath);
  const local = useEditor((s) => (path === undefined ? undefined : s.docs[path]?.content));
  const saved = useMemo(() => (local === undefined ? undefined : positionsOf(local)), [local]);
  const [ahead, setAhead] = useState<ReadonlyMap<string, Point> | null>(null);
  const [failed, setFailed] = useState(false);
  const creating = useRef(false);
  const waiting = useRef<ReadonlyMap<string, Point> | null>(null);
  const commit = useCallback(
    (positions: ReadonlyMap<string, Point>): boolean => {
      if (path === undefined || !canEdit) {
        return false;
      }
      setFailed(false);
      if (session.store.getState().docs[path] !== undefined) {
        session.edit(path, positionsRecipe(positions), { group: "screens" });
        setAhead(null);
        return true;
      }
      setAhead(positions);
      if (creating.current) {
        waiting.current = positions;
        return true;
      }
      creating.current = true;
      void session.create(path, positionsDocument(uuidv7(), positions)).then((r) => {
        creating.current = false;
        const later = waiting.current;
        waiting.current = null;
        if (!r.ok) {
          setFailed(true);
        } else if (later !== null) {
          session.edit(path, positionsRecipe(later), { group: "screens" });
        }
        setAhead(null);
      });
      return true;
    },
    [path, canEdit, session],
  );
  return { positions: ahead ?? saved, commit, failed };
}
