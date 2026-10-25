// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createContext, type ReactNode, use, useEffect } from "react";
import { type CommandState, chordOf, isEditable, type Registry } from "./registry.ts";

export interface CommandsValue {
  readonly registry: Registry;
  readonly states: readonly CommandState[];
}

const Context = createContext<CommandsValue | null>(null);

/** Provides the registry and installs the global shortcut handler (no events from inputs or contenteditable). */
export function CommandsProvider({ value, children }: { value: CommandsValue; children: ReactNode }) {
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const chord = chordOf(event);
      if (chord === undefined || event.defaultPrevented) {
        return;
      }
      const command = value.registry.byShortcut(chord);
      // Plain and shifted keys are typing in an input; chords with Control, Command or Alt still work there only for the palette.
      if (command === undefined || (isEditable(event.target) && command.id !== "palette.open")) {
        return;
      }
      const state = value.states.find((s) => s.command.id === command.id);
      if (state?.enabled !== true) {
        return;
      }
      event.preventDefault();
      void command.run();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [value]);
  return <Context value={value}>{children}</Context>;
}

export function useCommands(): CommandsValue {
  const value = use(Context);
  if (value === null) {
    throw new Error("useCommands outside CommandsProvider");
  }
  return value;
}

/** The commands when a provider surrounds the caller, else null (a view that works with or without the shell). */
export function useOptionalCommands(): CommandsValue | null {
  return use(Context);
}
