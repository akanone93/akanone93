// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type EditFn, nudgeRecipe } from "./edits.ts";
import type { ArrowKey } from "./geometry.ts";
import * as sel from "./selection.ts";
import type { SelectionStore } from "./store.ts";
import type { NodeIndex } from "./tree.ts";

export interface KeyEventLike {
  readonly key: string;
  readonly shiftKey: boolean;
  readonly ctrlKey: boolean;
  readonly metaKey: boolean;
}

export interface KeyContext {
  readonly store: SelectionStore;
  readonly index: NodeIndex;
  readonly edit: EditFn;
  /** The document that holds a node; nudges are grouped per key burst. */
  readonly docOf: (id: string) => string | undefined;
}

const ARROWS = new Set(["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"]);

function navigate(c: KeyContext, e: KeyEventLike, mod: boolean): sel.Selection | null {
  const s = c.store.get();
  if (e.key === "Escape") {
    return sel.toParent(c.index, s);
  }
  if (e.key === "Enter") {
    return sel.enter(c.index, s);
  }
  if (e.key === "Tab" && s.anchor !== null) {
    return sel.sibling(c.index, s, e.shiftKey ? -1 : 1);
  }
  return mod && e.key.toLowerCase() === "a" ? sel.selectSiblings(c.index, s) : null;
}

/** Keyboard operation of the canvas (WCAG 2.1.1): returns true when the key was handled. */
export function handleKey(c: KeyContext, e: KeyEventLike): boolean {
  const mod = e.ctrlKey || e.metaKey;
  const nav = navigate(c, e, mod);
  if (nav !== null) {
    c.store.set(nav);
    return true;
  }
  const s = c.store.get();
  if (ARROWS.has(e.key) && s.ids.length > 0) {
    for (const id of s.ids) {
      const doc = c.docOf(id);
      if (doc !== undefined) {
        c.edit(doc, nudgeRecipe(id, e.key as ArrowKey, e.shiftKey), { group: "nudge" });
      }
    }
    return true;
  }
  return false;
}
