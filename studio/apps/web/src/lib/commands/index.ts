// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

export { commandMessages } from "./commands.messages.ts";
export {
  CommandsProvider,
  type CommandsValue,
  useCommands,
  useOptionalCommands,
  useScopedCommands,
} from "./commands-context.tsx";
export {
  type Command,
  type CommandGroup,
  type CommandState,
  createRegistry,
  formatShortcut,
  mergeScoped,
  type Registry,
} from "./registry.ts";
export { shellCommands } from "./shell-commands.ts";
export {
  createUndoFocus,
  type UndoFocus,
  type UndoTarget,
  undoCommands,
} from "./undo-commands.ts";
