// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createContext, createElement, type ReactNode, useContext, useRef, useSyncExternalStore } from "react";
import { emptySelection, type Selection } from "./selection.ts";

/** A selection store: one per open canvas, created by the provider (no module-level state). */
export interface SelectionStore {
  get(): Selection;
  set(next: Selection): void;
  subscribe(listener: () => void): () => void;
}

export function createSelectionStore(initial: Selection = emptySelection): SelectionStore {
  let state = initial;
  const listeners = new Set<() => void>();
  return {
    get: () => state,
    set: (next) => {
      if (next !== state) {
        state = next;
        for (const l of [...listeners]) {
          l();
        }
      }
    },
    subscribe: (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
  };
}

const Context = createContext<SelectionStore | null>(null);

/** Gives the canvas, Layers tree and panels below it the same selection. */
export function SelectionProvider({ children }: { children: ReactNode }) {
  const ref = useRef<SelectionStore | null>(null);
  ref.current ??= createSelectionStore();
  return createElement(Context.Provider, { value: ref.current }, children);
}

export function useSelectionStore(): SelectionStore {
  const store = useContext(Context);
  if (store === null) {
    throw new Error("useSelectionStore needs a SelectionProvider");
  }
  return store;
}

export function useSelection(): Selection {
  const store = useSelectionStore();
  return useSyncExternalStore(store.subscribe, store.get);
}
