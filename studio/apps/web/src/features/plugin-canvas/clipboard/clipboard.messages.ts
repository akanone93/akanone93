// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of copy, cut, paste and the dependency report (STU-067, STU-055; D34). */
export function clipboardMessages(i18n: I18n) {
  return {
    copy: i18n._({
      id: "clipboard.copy",
      message: "Copy",
      comment: "Command that copies the selected elements to the clipboard.",
    }),
    cut: i18n._({
      id: "clipboard.cut",
      message: "Cut",
      comment: "Command that copies the selected elements to the clipboard and removes them.",
    }),
    paste: i18n._({
      id: "clipboard.paste",
      message: "Paste",
      comment: "Command that pastes elements from the clipboard onto the canvas.",
    }),
    reportTitle: i18n._({
      id: "clipboard.reportTitle",
      message: "Pasted elements need more than this plugin has",
      comment: "Title of the dialog that lists what a paste from another plugin or app depends on and lacks here.",
    }),
    reportIntro: i18n._({
      id: "clipboard.reportIntro",
      message:
        "Nothing is added silently. Choose what happens to each missing item; nothing is pasted until you confirm.",
      comment: "Intro of the paste dependency report.",
    }),
    functions: i18n._({
      id: "clipboard.functions",
      message: "Functions",
      comment: "Heading of the functions that the pasted elements call but the target plugin is not granted.",
    }),
    translations: i18n._({
      id: "clipboard.translations",
      message: "Translation keys",
      comment: "Heading of the translation keys that the pasted elements use but the target lacks.",
    }),
    assets: i18n._({
      id: "clipboard.assets",
      message: "Assets",
      comment: "Heading of the assets that the pasted elements use but the target lacks.",
    }),
    dataSources: i18n._({
      id: "clipboard.dataSources",
      message: "Data sources",
      comment: "Heading of the data sources that the pasted elements read but the target lacks.",
    }),
    pasteWithChoices: i18n._({
      id: "clipboard.pasteWithChoices",
      message: "Paste with these choices",
      comment: "Button that applies the paste with the choices made for the missing items.",
    }),
    actionMap: i18n._({
      id: "clipboard.actionMap",
      message: "Map to existing",
      comment: "Choice for a missing item in the paste report: use an existing item of the target instead.",
    }),
    actionCopy: i18n._({
      id: "clipboard.actionCopy",
      message: "Copy over",
      comment: "Choice for a missing item in the paste report: copy the item into the target.",
    }),
    actionDrop: i18n._({
      id: "clipboard.actionDrop",
      message: "Drop",
      comment: "Choice for a missing item in the paste report: remove the reference from the pasted elements.",
    }),
    actionFor: (id: string) =>
      i18n._({
        id: "clipboard.actionFor",
        message: "What to do with {id}",
        values: { id },
        comment: "Accessible name of the select that chooses what happens to one missing item. {id} is its ID.",
      }),
    mapTarget: (id: string) =>
      i18n._({
        id: "clipboard.mapTarget",
        message: "Existing item to use for {id}",
        values: { id },
        comment: "Accessible name of the select that picks the existing item a missing one is mapped to.",
      }),
    cancel: i18n._({
      id: "clipboard.cancel",
      message: "Cancel",
      comment: "Button that cancels the paste.",
    }),
    close: i18n._({
      id: "clipboard.close",
      message: "Close",
      comment: "Accessible name of the button that closes the paste dialog.",
    }),
    refusedTitle: i18n._({
      id: "clipboard.refusedTitle",
      message: "The clipboard was not pasted",
      comment: "Title of the dialog that tells the clipboard content was refused.",
    }),
    refusedTooLarge: i18n._({
      id: "clipboard.refusedTooLarge",
      message: "The clipboard holds more than the size limit allows.",
      comment: "Why a paste was refused: the content is over the size limit.",
    }),
    refusedVersion: i18n._({
      id: "clipboard.refusedVersion",
      message: "The clipboard comes from a newer or unknown version of Studio.",
      comment: "Why a paste was refused: the envelope version is unknown.",
    }),
    refusedMalformed: i18n._({
      id: "clipboard.refusedMalformed",
      message: "The clipboard content is damaged.",
      comment: "Why a paste was refused: the envelope is malformed.",
    }),
    refusedInvalid: i18n._({
      id: "clipboard.refusedInvalid",
      message: "The pasted elements do not pass validation.",
      comment: "Why a paste was refused: the validator rejected the elements.",
    }),
    droppedTitle: i18n._({
      id: "clipboard.droppedTitle",
      message: "Some items were not copied",
      comment: "Title of the dialog that lists the items a paste could not copy over.",
    }),
    droppedIntro: i18n._({
      id: "clipboard.droppedIntro",
      message: "These items were left out. Pasted elements no longer refer to them.",
      comment: "Explains the list of items that were not copied over by a paste.",
    }),
    dropForbidden: i18n._({
      id: "clipboard.dropForbidden",
      message: "You cannot read it.",
      comment: "Why an item was not copied over: the user has no read access to the source.",
    }),
    dropLimit: i18n._({
      id: "clipboard.dropLimit",
      message: "The change set is full.",
      comment: "Why an item was not copied over: the change-set document limit was reached.",
    }),
    dropRejected: i18n._({
      id: "clipboard.dropRejected",
      message: "The server refused it.",
      comment: "Why an item was not copied over: the server rejected the new document.",
    }),
  };
}
