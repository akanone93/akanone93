// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createContext, type ReactNode, use, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { type Command, type CommandState, chordOf, isEditable, mergeScoped, type Registry } from "./registry.ts";

export interface CommandsValue {
  readonly registry: Registry;
  readonly states: readonly CommandState[];
}

const Context = createContext<CommandsValue | null>(null);
type Register = (key: string, commands: readonly Command[]) => () => void;
const RegisterContext = createContext<Register | null>(null);

/** Provides the registry and installs the global shortcut handler (no events from inputs or contenteditable). */
export function CommandsProvider({ value: base, children }: { value: CommandsValue; children: ReactNode }) {
  const [scopes, setScopes] = useState<ReadonlyMap<string, readonly Command[]>>(new Map());
  const latest = useRef({ scopes, registry: base.registry });
  latest.current = { scopes, registry: base.registry };
  const register = useCallback<Register>((key, commands) => {
    // Throws on a duplicate id or a shortcut collision with the global or another scoped command.
    const next = new Map(latest.current.scopes).set(key, commands);
    mergeScoped(latest.current.registry, [...next.values()].flat());
    setScopes((old) => new Map(old).set(key, commands));
    return () =>
      setScopes((old) => {
        const rest = new Map(old);
        rest.delete(key);
        return rest;
      });
  }, []);
  const value = useMemo<CommandsValue>(() => {
    const scoped = [...scopes.values()].flat();
    if (scoped.length === 0) {
      return base;
    }
    return {
      registry: mergeScoped(base.registry, scoped),
      states: [
        ...base.states,
        ...scoped.map((command) =>
          command.disabledReason === undefined
            ? { command, enabled: command.permission === null }
            : { command, enabled: false, reason: command.disabledReason },
        ),
      ],
    };
  }, [base, scopes]);
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
  return (
    <RegisterContext value={register}>
      <Context value={value}>{children}</Context>
    </RegisterContext>
  );
}

export function useCommands(): CommandsValue {
  const value = use(Context);
  if (value === null) {
    throw new Error("useCommands outside CommandsProvider");
  }
  return value;
}

/** What the palette shows of a command: registering again is needed only when this changes. */
function shown(commands: readonly Command[]): string {
  return JSON.stringify(
    commands.map((c) => [c.id, c.title, c.group, c.shortcut ?? "", c.permission, c.disabledReason ?? null]),
  );
}

/**
 * Registers \`commands\` while the calling component is mounted (page-scoped commands). A registration re-renders
 * every reader of the commands, so the caller's list is registered again only when what it shows changes; each
 * \`run\` is called through the latest list, so a caller that builds new closures every render neither loops nor
 * runs a stale one.
 */
export function useScopedCommands(key: string, commands: readonly Command[]): void {
  const register = use(RegisterContext);
  const latest = useRef(commands);
  latest.current = commands;
  const signature = shown(commands);
  // biome-ignore lint/correctness/useExhaustiveDependencies: rebuilt only when what the palette shows changes
  const stable = useMemo<readonly Command[]>(
    () =>
      latest.current.map((c) => ({
        ...c,
        run: () => (latest.current.find((now) => now.id === c.id) ?? c).run(),
      })),
    [signature],
  );
  useEffect(() => (register === null ? undefined : register(key, stable)), [register, key, stable]);
}

/** The commands when a provider surrounds the caller, else null: a reader for views that also render without the shell. */
export function useOptionalCommands(): CommandsValue | null {
  return use(Context);
}
