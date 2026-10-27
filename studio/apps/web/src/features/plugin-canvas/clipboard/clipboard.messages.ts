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
      message: "Nothing is added silently. Here is what Studio does for each missing item.",
      comment: "Intro of the paste dependency report.",
    }),
    functions: i18n._({
      id: "clipboard.functions",
      message: "Functions",
      comment: "Heading of the functions that the pasted elements call but the target plugin is not granted.",
    }),
    functionsAction: i18n._({
      id: "clipboard.functionsAction",
      message: "The calls stay. They fail validation until the function is granted to this plugin.",
      comment: "What Studio does about functions the target lacks.",
    }),
    translations: i18n._({
      id: "clipboard.translations",
      message: "Translation keys",
      comment: "Heading of the translation keys that the pasted elements use but the target lacks.",
    }),
    translationsAction: i18n._({
      id: "clipboard.translationsAction",
      message: "The keys stay. Add their translations in Localisation.",
      comment: "What Studio does about translation keys the target lacks.",
    }),
    assets: i18n._({
      id: "clipboard.assets",
      message: "Assets",
      comment: "Heading of the assets that the pasted elements use but the target lacks.",
    }),
    assetsAction: i18n._({
      id: "clipboard.assetsAction",
      message: "The references stay and show a placeholder until you add the assets.",
      comment: "What Studio does about assets the target lacks.",
    }),
    dataSources: i18n._({
      id: "clipboard.dataSources",
      message: "Data sources",
      comment: "Heading of the data sources that the pasted elements read but the target lacks.",
    }),
    dataSourcesAction: i18n._({
      id: "clipboard.dataSourcesAction",
      message: "The bindings stay. They are flagged as problems until the data source exists.",
      comment: "What Studio does about data sources the target lacks.",
    }),
    pasteAnyway: i18n._({
      id: "clipboard.pasteAnyway",
      message: "Paste anyway",
      comment: "Button that applies the paste although dependencies are missing.",
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
  };
}
