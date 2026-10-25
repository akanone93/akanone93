// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";
import { createContext, useContext } from "react";
import { commandMessages } from "./commands.messages.ts";
import type { Command } from "./registry.ts";

/** What undo and redo act on: the stack of the focused editor (F3, STU-069). */
export interface UndoTarget {
  undo(): void;
  redo(): void;
}

/** The shell's slot for the focused editor's undo target; an editor sets and clears `current`. */
export interface UndoFocus {
  current: UndoTarget | undefined;
}

export function createUndoFocus(): UndoFocus {
  return { current: undefined };
}

const UndoFocusContext = createContext<UndoFocus | null>(null);

/** Provides the shell's undo slot to the editors below it. */
export const UndoFocusProvider = UndoFocusContext.Provider;

/** The shell's undo slot, or null outside the shell. */
export function useUndoFocus(): UndoFocus | null {
  return useContext(UndoFocusContext);
}

/**
 * Undo (mod+z) and redo (shift+mod+z, and mod+y) through the command registry. With no editor
 * focused they do nothing; an overlay code editor keeps its own undo while it has focus (F3).
 */
export function undoCommands(i18n: I18n, focus: UndoFocus): readonly Command[] {
  const m = commandMessages(i18n);
  return [
    {
      id: "edit.undo",
      title: m.undo,
      group: "edit",
      shortcut: "mod+z",
      permission: null,
      run: () => focus.current?.undo(),
    },
    {
      id: "edit.redo",
      title: m.redo,
      group: "edit",
      shortcut: "mod+shift+z",
      permission: null,
      run: () => focus.current?.redo(),
    },
    {
      id: "edit.redo.alt",
      title: m.redoAlt,
      group: "edit",
      shortcut: "mod+y",
      permission: null,
      run: () => focus.current?.redo(),
    },
  ];
}
