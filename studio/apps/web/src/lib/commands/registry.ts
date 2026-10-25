// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { PermissionName } from "@plux/schema";

export type CommandGroup = "navigate" | "organisation" | "account" | "appearance";

/**
 * A palette command (STU-013, M-7). `permission` names the server permission that guards the RPC the command
 * calls, typed against the generated catalogue so an unknown name does not compile; `null` states that the
 * command needs none (pure navigation or an RPC open to every signed-in person).
 */
export interface Command {
  readonly id: string;
  readonly title: string;
  readonly group: CommandGroup;
  /** Default chord such as `mod+k` or `alt+h`; `mod` is Command on macOS and Control elsewhere. */
  readonly shortcut?: string;
  readonly permission: PermissionName | null;
  readonly run: () => void | Promise<void>;
}

export interface CommandState {
  readonly command: Command;
  readonly enabled: boolean;
  /** Set when disabled: the missing permission, never a silent hiding (F14). */
  readonly missing?: PermissionName;
}

const modifierOrder = ["mod", "alt", "shift"] as const;

/** Canonical form of a chord: modifiers in fixed order, key lower-case. */
export function normaliseShortcut(shortcut: string): string {
  const parts = shortcut.toLowerCase().split("+");
  const key = parts.at(-1) ?? "";
  const mods = modifierOrder.filter((m) => parts.includes(m));
  return [...mods, key].join("+");
}

export interface Registry {
  readonly commands: readonly Command[];
  stateFor(permissions: readonly string[]): readonly CommandState[];
  byShortcut(chord: string): Command | undefined;
}

/** Builds the registry; a duplicate id or a shortcut collision is a programming error and throws. */
export function createRegistry(commands: readonly Command[]): Registry {
  const ids = new Set<string>();
  const chords = new Map<string, Command>();
  for (const command of commands) {
    if (ids.has(command.id)) {
      throw new Error(`duplicate command id: ${command.id}`);
    }
    ids.add(command.id);
    if (command.shortcut !== undefined) {
      const chord = normaliseShortcut(command.shortcut);
      const other = chords.get(chord);
      if (other !== undefined) {
        throw new Error(`shortcut ${chord} is used by ${other.id} and ${command.id}`);
      }
      chords.set(chord, command);
    }
  }
  return {
    commands,
    stateFor: (permissions) =>
      commands.map((command) =>
        command.permission === null || permissions.includes(command.permission)
          ? { command, enabled: true }
          : { command, enabled: false, missing: command.permission },
      ),
    byShortcut: (chord) => chords.get(normaliseShortcut(chord)),
  };
}

/** The chord of a keyboard event in registry form, or undefined for a bare modifier press. */
export function chordOf(
  event: Pick<KeyboardEvent, "key" | "code" | "ctrlKey" | "metaKey" | "altKey" | "shiftKey">,
): string | undefined {
  if (["Control", "Meta", "Alt", "Shift"].includes(event.key)) {
    return undefined;
  }
  // Alt rewrites the character on some layouts, so the physical key identifies the chord.
  const key = event.altKey && event.code.startsWith("Key") ? event.code.slice(3) : event.key;
  const parts: string[] = [];
  if (event.ctrlKey || event.metaKey) {
    parts.push("mod");
  }
  if (event.altKey) {
    parts.push("alt");
  }
  if (event.shiftKey) {
    parts.push("shift");
  }
  parts.push(key.toLowerCase());
  return parts.join("+");
}

/** Human form of a chord for display, with ⌘ on Apple platforms. */
export function formatShortcut(shortcut: string, apple: boolean): string {
  const names: Readonly<Record<string, readonly [string, string]>> = {
    mod: ["⌘", "Ctrl"],
    alt: ["⌥", "Alt"],
    shift: ["Shift", "Shift"],
  };
  return normaliseShortcut(shortcut)
    .split("+")
    .map((p) => names[p]?.[apple ? 0 : 1] ?? p.toUpperCase())
    .join(apple ? "" : "+");
}

/** True for targets where typing must not trigger a shortcut. */
export function isEditable(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false;
  }
  return (
    target.isContentEditable ||
    target.contentEditable === "true" ||
    ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
  );
}
