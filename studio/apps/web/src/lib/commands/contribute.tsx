// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createContext, type ReactNode, useContext, useEffect, useMemo, useState } from "react";
import type { Command } from "./registry.ts";

type Contribute = (owner: string, commands: readonly Command[]) => void;

const Context = createContext<Contribute | null>(null);

/** Collects the commands that mounted features contribute (a route's own commands), for the shell's registry. */
export function useContributions(): {
  readonly contributed: readonly Command[];
  readonly Provider: (p: { children: ReactNode }) => ReactNode;
} {
  const [byOwner, setByOwner] = useState<ReadonlyMap<string, readonly Command[]>>(new Map());
  const contribute = useMemo<Contribute>(
    () => (owner, commands) =>
      setByOwner((old) => {
        const next = new Map(old);
        if (commands.length === 0) {
          next.delete(owner);
        } else {
          next.set(owner, commands);
        }
        return next;
      }),
    [],
  );
  const contributed = useMemo(() => [...byOwner.values()].flat(), [byOwner]);
  const Provider = useMemo(
    () =>
      function ContributionProvider({ children }: { children: ReactNode }) {
        return <Context value={contribute}>{children}</Context>;
      },
    [contribute],
  );
  return { contributed, Provider };
}

/** Offers commands to the palette while the calling component is mounted; removed on unmount. */
export function useContributeCommands(owner: string, commands: readonly Command[]): void {
  const contribute = useContext(Context);
  useEffect(() => {
    contribute?.(owner, commands);
    return () => contribute?.(owner, []);
  }, [contribute, owner, commands]);
}
