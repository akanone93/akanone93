// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

export {
  applyChangeSet,
  type ChangeSet,
  type ChangeSetDraft,
  type ChangeSetEdit,
  type DraftEntry,
  type MoveResult,
  moveScreen,
  redirectIncoming,
  redirectPluginIncoming,
  removeOutgoing,
  undoStep,
} from "./change-set.ts";
export { loadedDocs, type PluginRef, pluginRefs } from "./loaded.ts";
export { BlockedDeletePanel, RenameWarning, TargetPicker, TypeToConfirmDialog } from "./safety-dialogs.tsx";
export { type OpenPlugin, useChangeSets, useProductionRelease } from "./use-safety.ts";
export {
  buildWhereUsed,
  camel,
  checkEntityDelete,
  checkPluginDelete,
  checkScreenDelete,
  checkStateDelete,
  componentKeyEffects,
  type DeleteCheck,
  isBlocked,
  type LoadedDoc,
  type PageInfo,
  type RenameEffect,
  routeChangeEffects,
  type Use,
  type UseKind,
  type WhereUsedIndex,
} from "./where-used.ts";
