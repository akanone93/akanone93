// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";
import { commandMessages } from "./commands.messages.ts";
import type { Command } from "./registry.ts";

/** What undo and redo act on: the stack of the focused editor (F3, STU-069). */
export interface UndoTarget {
  undo(): void;
  redo(): void;
}

/** The shell's slot for the focused editor's undo target; an editor sets and clears it. */
export interface UndoFocus {
  readonly current: UndoTarget | undefined;
  set(target: UndoTarget | undefined): void;
  /** For the shell, which registers the undo commands only while an editor fills the slot. */
  subscribe(listener: () => void): () => void;
}

export function createUndoFocus(): UndoFocus {
  let current: UndoTarget | undefined;
  const listeners = new Set<() => void>();
  return {
    get current() {
      return current;
    },
    set(target) {
      current = target;
      for (const l of listeners) {
        l();
      }
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}

/**
 * Undo (mod+z) and redo (shift+mod+z, and mod+y) through the command registry. With no editor
 * focused they do nothing; an overlay code editor keeps its own undo while it has focus (F3).
 */
export function undoCommands(i18n: I18n, focus: UndoFocus, readOnly = false): readonly Command[] {
  const m = commandMessages(i18n);
  const blocked = readOnly ? { disabledReason: m.readOnly } : {};
  return [
    {
      id: "edit.undo",
      title: m.undo,
      group: "edit",
      shortcut: "mod+z",
      permission: null,
      ...blocked,
      run: () => focus.current?.undo(),
    },
    {
      id: "edit.redo",
      title: m.redo,
      group: "edit",
      shortcut: "mod+shift+z",
      permission: null,
      ...blocked,
      run: () => focus.current?.redo(),
    },
    {
      id: "edit.redo.alt",
      title: m.redoAlt,
      group: "edit",
      shortcut: "mod+y",
      permission: null,
      ...blocked,
      run: () => focus.current?.redo(),
    },
  ];
}
