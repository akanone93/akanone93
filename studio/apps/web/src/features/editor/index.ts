// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

export {
  classifySaveError,
  documentTransport,
  loadSessionEntries,
  tabEditorSession,
  toServerDocument,
} from "./document-transport.ts";
export {
  type Conflict,
  type DocState,
  type DocStatus,
  type DocumentDraft,
  type DocumentTransport,
  type EditRecord,
  EditSession,
  type EditSessionOptions,
  type EditState,
  type SaveFailure,
  type Scheduler,
  type ServerDocument,
} from "./edit-session.ts";
export {
  type AcquireLock,
  EditorProvider,
  type EditorProviderProps,
  useEditor,
  useEditSession,
} from "./editor-provider.tsx";
export { applyPatch, type JsonPatchOp, type JsonValue, PatchError, toJsonPatch } from "./json-patch.ts";
export {
  createPendingCounter,
  PendingChangesProvider,
  type PendingCounter,
  usePendingTotal,
  useReportPending,
} from "./pending-changes.tsx";
export {
  pointerSegments,
  type SessionEntry,
  toImmerPatches,
  type UndoBlock,
  UndoManager,
  type UndoManagerOptions,
  type UndoResult,
} from "./undo-stack.ts";
