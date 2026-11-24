// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { useMemo } from "react";
import { createUndoFocus, undoCommands, useScopedCommands } from "../../lib/commands/index.ts";
import { useCanEdit } from "./editor-provider.tsx";
import type { UndoManager } from "./undo-stack.ts";

/**
 * Registers edit.undo and edit.redo (mod+z, mod+shift+z) for one undo stack — a plugin's, or the app's own documents'
 * ("") on the app canvas — disabled with the reason while the editor is read-only (F3, STU-069). `canEdit` is kept
 * current for the manager's `blocked` check.
 */
export function UndoCommands(props: {
  readonly undo: UndoManager;
  readonly stack: string;
  readonly canEdit: { current: boolean };
  /** The command scope: one per page. */
  readonly scope: string;
}): null {
  const { i18n } = useLingui();
  const canEdit = useCanEdit();
  props.canEdit.current = canEdit;
  const { undo, stack } = props;
  const list = useMemo(() => {
    const focus = createUndoFocus();
    focus.set({ undo: () => undo.undo(stack), redo: () => undo.redo(stack) });
    return undoCommands(i18n, focus, !canEdit);
  }, [i18n, undo, stack, canEdit]);
  useScopedCommands(props.scope, list);
  return null;
}
