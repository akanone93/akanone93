// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the edit pipeline: the D35 conflict dialog and the waiting-changes chip (decisions F2, F4). */
export function editorMessages(i18n: I18n) {
  return {
    conflictTitle: i18n._({
      id: "editor.conflict.title",
      message: "This plugin changed on the server",
      comment: "D35 title: someone or something saved this plugin while you were editing it.",
    }),
    conflictBody: i18n._({
      id: "editor.conflict.body",
      message: "Your changes could not be combined with the newer version. Autosave is paused until you choose.",
      comment: "D35 explanation shown under the title.",
    }),
    conflictDifferences: i18n._({
      id: "editor.conflict.differences",
      message: "Where your version and the server's differ",
      comment: "D35 heading of the list of document paths that differ.",
    }),
    conflictWouldDrop: i18n._({
      id: "editor.conflict.would-drop",
      message: "Keeping yours cannot apply these changes",
      comment: "D35 heading of the list of your changes that no longer fit the server's version.",
    }),
    conflictDropped: i18n._({
      id: "editor.conflict.dropped",
      message: "These changes of yours were not applied",
      comment: "Shown after keep mine: the changes that did not fit and were left out.",
    }),
    keepMine: i18n._({
      id: "editor.conflict.keep-mine",
      message: "Keep mine",
      comment: "D35 button: apply your changes on top of the server's version where they fit.",
    }),
    takeServer: i18n._({
      id: "editor.conflict.take-server",
      message: "Take the server's",
      comment: "D35 button: discard your unsaved changes and use the server's version.",
    }),
    copyMine: i18n._({
      id: "editor.conflict.copy-mine",
      message: "Copy mine as JSON",
      comment: "D35 button: copy your version of the document to the clipboard before choosing.",
    }),
    copied: i18n._({
      id: "editor.conflict.copied",
      message: "Copied to the clipboard",
      comment: "Confirmation after copying your version as JSON.",
    }),
    close: i18n._({
      id: "editor.conflict.close",
      message: "Close",
      comment: "Accessible name of the D35 close button.",
    }),
    waiting: (count: number) =>
      i18n._({
        id: "chip.waiting",
        message: "{count, plural, one {# change waiting} other {# changes waiting}}",
        values: { count },
        comment: "Status chip: edits not yet saved, kept in memory until the server is reachable.",
      }),
    offlineWaiting: (count: number) =>
      i18n._({
        id: "chip.offline-waiting",
        message: "{count, plural, one {Offline — # change waiting} other {Offline — # changes waiting}}",
        values: { count },
        comment: "Status chip: no network, and edits wait in memory to be sent on reconnect.",
      }),
  };
}
