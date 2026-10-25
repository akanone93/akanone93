// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createContext, type ReactElement, type ReactNode, useContext, useEffect, useSyncExternalStore } from "react";

/** Counts unsaved edits per open editor area for the shell's E13 chip (decisions F4). */
export interface PendingCounter {
  set(key: string, count: number): void;
  total(): number;
  subscribe(listener: () => void): () => void;
}

export function createPendingCounter(): PendingCounter {
  const counts = new Map<string, number>();
  const listeners = new Set<() => void>();
  let total = 0;
  return {
    set(key, count) {
      if (count === 0) {
        counts.delete(key);
      } else {
        counts.set(key, count);
      }
      const next = [...counts.values()].reduce((a, b) => a + b, 0);
      if (next !== total) {
        total = next;
        for (const l of listeners) {
          l();
        }
      }
    },
    total: () => total,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}

const PendingContext = createContext<PendingCounter | null>(null);

export function PendingChangesProvider({
  counter,
  children,
}: {
  readonly counter: PendingCounter;
  readonly children: ReactNode;
}): ReactElement {
  return <PendingContext.Provider value={counter}>{children}</PendingContext.Provider>;
}

/** The number of edits waiting across the open editors. */
export function usePendingTotal(counter: PendingCounter): number {
  return useSyncExternalStore(counter.subscribe, counter.total, counter.total);
}

/** Reports one editor's waiting edits to the shell, if a shell is present. */
export function useReportPending(key: string, count: number): void {
  const counter = useContext(PendingContext);
  useEffect(() => {
    counter?.set(key, count);
  }, [counter, key, count]);
  useEffect(() => () => counter?.set(key, 0), [counter, key]);
}
