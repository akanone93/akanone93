// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the history overlay (S5.6, STU-069). */
export function historyMessages(i18n: I18n) {
  return {
    open: i18n._({
      id: "history.open",
      message: "History",
      comment: "Button in the right panel that opens the history of the plugin.",
    }),
    title: i18n._({
      id: "history.title",
      message: "History",
      comment: "Title of the full-screen overlay that lists the snapshots of the plugin.",
    }),
    close: i18n._({
      id: "history.close",
      message: "Close the history",
      comment: "Accessible name of the button that closes the history overlay.",
    }),
    list: i18n._({
      id: "history.list",
      message: "Snapshots",
      comment: "Accessible name of the list of snapshots.",
    }),
    empty: i18n._({
      id: "history.empty",
      message: "There are no snapshots yet.",
      comment: "Shown when the plugin has no snapshots.",
    }),
    loading: i18n._({
      id: "history.loading",
      message: "Loading the snapshots.",
      comment: "Shown while the snapshots load.",
    }),
    failed: i18n._({
      id: "history.failed",
      message: "The snapshots could not be loaded.",
      comment: "Shown when the list of snapshots could not be loaded.",
    }),
    more: i18n._({
      id: "history.more",
      message: "Load more",
      comment: "Button that loads the next page of snapshots.",
    }),
    pick: i18n._({
      id: "history.pick",
      message: "Choose for comparison",
      comment: "Label prefix of the checkbox that picks a snapshot for comparison.",
    }),
    pickRow: (when: string) =>
      i18n._({
        id: "history.pickRow",
        message: "Compare {when}",
        values: { when },
        comment: "Accessible name of the checkbox of one snapshot; the placeholder is its time.",
      }),
    restore: i18n._({
      id: "history.restore",
      message: "Restore",
      comment: "Button that restores one snapshot.",
    }),
    restoreRow: (when: string) =>
      i18n._({
        id: "history.restoreRow",
        message: "Restore the snapshot of {when}",
        values: { when },
        comment: "Accessible name of the restore button of one snapshot; the placeholder is its time.",
      }),
    by: (author: string) =>
      i18n._({
        id: "history.by",
        message: "by {author}",
        values: { author },
        comment: "Author of a snapshot, after its time; the placeholder is a person's name.",
      }),
    unknownAuthor: i18n._({
      id: "history.unknownAuthor",
      message: "unknown author",
      comment: "Shown when the author of a snapshot is not known.",
    }),
    reasonWrite: i18n._({
      id: "history.reasonWrite",
      message: "Edit",
      comment: "Why a snapshot was made: an edit was saved.",
    }),
    reasonLockTakeover: i18n._({
      id: "history.reasonLockTakeover",
      message: "Lock taken over",
      comment: "Why a snapshot was made: someone took over the editing lock.",
    }),
    reasonPublish: i18n._({
      id: "history.reasonPublish",
      message: "Published",
      comment: "Why a snapshot was made: the plugin was published.",
    }),
    reasonRestore: i18n._({
      id: "history.reasonRestore",
      message: "Restored",
      comment: "Why a snapshot was made: an earlier snapshot was restored.",
    }),
    compareHint: i18n._({
      id: "history.compareHint",
      message: "Choose two snapshots to compare them.",
      comment: "Shown while fewer than two snapshots are chosen for comparison.",
    }),
    compareTitle: i18n._({
      id: "history.compareTitle",
      message: "Differences between the two snapshots",
      comment: "Heading of the list of documents that differ between two snapshots.",
    }),
    noChanges: i18n._({
      id: "history.noChanges",
      message: "The two snapshots are the same.",
      comment: "Shown when two compared snapshots hold the same documents.",
    }),
    changeAdded: i18n._({
      id: "history.changeAdded",
      message: "added",
      comment: "Tag of a document that exists only in the newer of two compared snapshots.",
    }),
    changeRemoved: i18n._({
      id: "history.changeRemoved",
      message: "removed",
      comment: "Tag of a document that exists only in the older of two compared snapshots.",
    }),
    changeChanged: i18n._({
      id: "history.changeChanged",
      message: "changed",
      comment: "Tag of a document that differs between two compared snapshots.",
    }),
    document: i18n._({
      id: "history.document",
      message: "Document",
      comment: "Label of the list of documents that differ between two snapshots.",
    }),
    diffLabel: i18n._({
      id: "history.diffLabel",
      message: "Older and newer version of the document",
      comment: "Accessible name of the side-by-side view of the two versions of a document.",
    }),
    diffFailed: i18n._({
      id: "history.diffFailed",
      message: "The documents could not be loaded.",
      comment: "Shown when the two versions of a document could not be loaded.",
    }),
    confirmTitle: i18n._({
      id: "history.confirmTitle",
      message: "Restore this snapshot?",
      comment: "Title of the confirmation before a snapshot is restored.",
    }),
    confirmBody: (when: string) =>
      i18n._({
        id: "history.confirmBody",
        message: "The plugin's documents return to how they were at {when}. You can undo the restore in one step.",
        values: { when },
        comment: "Body of the confirmation before a snapshot is restored; the placeholder is the snapshot's time.",
      }),
    cancel: i18n._({
      id: "history.cancel",
      message: "Cancel",
      comment: "Button that cancels the restore.",
    }),
    confirm: i18n._({
      id: "history.confirm",
      message: "Restore",
      comment: "Button that confirms the restore.",
    }),
    restored: (count: number) =>
      i18n._({
        id: "history.restored",
        message: "{count, plural, one {Restored # document.} other {Restored # documents.}}",
        values: { count },
        comment: "Result after a restore; the count is the number of documents that changed.",
      }),
    restoredNothing: i18n._({
      id: "history.restoredNothing",
      message: "Nothing changed: the plugin already matches this snapshot.",
      comment: "Result after a restore that changed nothing.",
    }),
    restoreFailed: i18n._({
      id: "history.restoreFailed",
      message: "The snapshot could not be restored.",
      comment: "Shown when restoring a snapshot failed.",
    }),
    readOnly: i18n._({
      id: "history.readOnly",
      message: "Someone else is editing this plugin, so snapshots cannot be restored.",
      comment: "Shown when the editor is read-only.",
    }),
  };
}

export type HistoryMessages = ReturnType<typeof historyMessages>;
