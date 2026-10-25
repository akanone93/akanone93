// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useMemo } from "react";
import type { StoreApi } from "zustand/vanilla";
import { useEditSession } from "../../editor/editor-provider.tsx";
import type { Selection } from "./selection.ts";

/** The selection as the interactions see it: a view of the area's Zustand store (F5, ADR-0014). */
export interface SelectionStore {
  get(): Selection;
  set(next: Selection): void;
}

/** Adapts any store that holds a `selection` (the editor's area store, or a test store). */
export function selectionStoreOf(api: StoreApi<{ readonly selection: Selection }>): SelectionStore {
  return {
    get: () => api.getState().selection,
    set: (next) => {
      if (next !== api.getState().selection) {
        api.setState({ selection: next });
      }
    },
  };
}

/** The selection store of the surrounding EditorProvider. */
export function useSelectionStore(): SelectionStore {
  const api = useEditSession().store;
  return useMemo(() => selectionStoreOf(api), [api]);
}
