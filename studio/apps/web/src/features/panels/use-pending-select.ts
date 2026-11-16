// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useCallback, useEffect, useRef } from "react";
import type { SelectionStore } from "../plugin-canvas/select/store.ts";
import type { NodeIndex } from "../plugin-canvas/select/tree.ts";
import { selectIds } from "./selection-model.ts";

/**
 * Selects a widget that was just inserted (PC-04). The edit reaches the node index a render later, and a selection of
 * an id the index does not hold yet is ignored, so the id waits until the index has it; then it is selected and
 * `onSelected` runs (to bring it into view). Without this, an inserted widget was never selected and one that draws
 * nothing on its own (an empty Dismissible, a button without a label) left no trace on the canvas.
 */
export function usePendingSelect(
  index: NodeIndex,
  store: SelectionStore,
  onSelected?: (id: string) => void,
): (id: string) => void {
  const pending = useRef<string | null>(null);
  const after = useRef(onSelected);
  after.current = onSelected;
  const choose = useCallback(
    (id: string) => {
      pending.current = null;
      store.set(selectIds(index, store.get(), id, false));
      after.current?.(id);
    },
    [index, store],
  );
  useEffect(() => {
    const id = pending.current;
    if (id !== null && index.parent.has(id)) {
      choose(id);
    }
  }, [index, choose]);
  return useCallback(
    (id: string) => {
      if (index.parent.has(id)) {
        choose(id);
      } else {
        pending.current = id;
      }
    },
    [index, choose],
  );
}
