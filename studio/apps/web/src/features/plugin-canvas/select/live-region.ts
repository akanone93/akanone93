// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Selection } from "./selection.ts";

/** What the polite live region says about a selection (F5); `name` resolves a node ID to its label. */
export interface Announcer {
  none(): string;
  one(name: string): string;
  many(count: number): string;
}

export function announce(a: Announcer, s: Selection, name: (id: string) => string): string {
  const first = s.ids[0];
  if (first === undefined) {
    return a.none();
  }
  return s.ids.length === 1 ? a.one(name(first)) : a.many(s.ids.length);
}
