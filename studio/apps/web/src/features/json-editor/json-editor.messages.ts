// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the JSON editor overlay (S5.1, STU-064). */
export function jsonEditorMessages(i18n: I18n) {
  return {
    open: i18n._({
      id: "jsonEditor.open",
      message: "JSON editor",
      comment: "Button in the right panel that opens the full-screen JSON editor for the selected document.",
    }),
    title: i18n._({
      id: "jsonEditor.title",
      message: "JSON editor",
      comment: "Title of the full-screen overlay in which a document's JSON is edited.",
    }),
    close: i18n._({
      id: "jsonEditor.close",
      message: "Close the JSON editor",
      comment: "Accessible name of the button that closes the full-screen JSON editor.",
    }),
    document: i18n._({
      id: "jsonEditor.document",
      message: "Document",
      comment: "Label of the list from which the document to edit is chosen.",
    }),
    editorLabel: i18n._({
      id: "jsonEditor.editorLabel",
      message: "JSON of the document",
      comment: "Accessible name of the text editor that holds a document's JSON.",
    }),
    diff: i18n._({
      id: "jsonEditor.diff",
      message: "Compare with the last published version",
      comment: "Toggle that shows the document next to the version that was last published.",
    }),
    noPublished: i18n._({
      id: "jsonEditor.noPublished",
      message: "This document has not been published yet, so there is nothing to compare with.",
      comment: "Shown instead of the comparison when the plugin has no published version.",
    }),
    loadingPublished: i18n._({
      id: "jsonEditor.loadingPublished",
      message: "Loading the published version…",
      comment: "Shown while the last published version of the document is fetched for the comparison.",
    }),
    publishedFailed: i18n._({
      id: "jsonEditor.publishedFailed",
      message: "The published version could not be loaded.",
      comment: "Shown when fetching the last published version for the comparison failed.",
    }),
    apply: i18n._({
      id: "jsonEditor.apply",
      message: "Apply",
      comment: "Button that applies the edited JSON to the document. Enabled only when the JSON is valid.",
    }),
    applied: i18n._({
      id: "jsonEditor.applied",
      message: "Applied to the document.",
      comment: "Confirmation after the edited JSON was applied to the document.",
    }),
    nothingToApply: i18n._({
      id: "jsonEditor.nothingToApply",
      message: "No changes to apply.",
      comment: "Status when the edited JSON equals the document.",
    }),
    checking: i18n._({
      id: "jsonEditor.checking",
      message: "Checking…",
      comment: "Status while the validator checks the edited JSON.",
    }),
    valid: i18n._({
      id: "jsonEditor.valid",
      message: "Valid. You can apply it.",
      comment: "Status when the edited JSON passed validation.",
    }),
    invalid: i18n._({
      id: "jsonEditor.invalid",
      message: "Fix the problems below to apply.",
      comment: "Status when the edited JSON has errors, so it cannot be applied.",
    }),
    unavailable: i18n._({
      id: "jsonEditor.unavailable",
      message: "The validator is not ready, so the JSON cannot be applied yet.",
      comment: "Status when the validator has not finished loading, so applying is not possible.",
    }),
    readOnly: i18n._({
      id: "jsonEditor.readOnly",
      message: "You cannot edit this plugin now, so the JSON is read-only.",
      comment: "Shown when the editor is read-only (no lock on the plugin or insufficient permission).",
    }),
    problems: i18n._({
      id: "jsonEditor.problems",
      message: "Problems",
      comment: "Heading of the list of validation findings for the edited JSON.",
    }),
    noProblems: i18n._({
      id: "jsonEditor.noProblems",
      message: "No problems found.",
      comment: "Shown in the list of validation findings when there are none.",
    }),
    syntax: (message: string) =>
      i18n._({
        id: "jsonEditor.syntax",
        message: "Not valid JSON: {message}",
        values: { message },
        comment: "Finding shown when the text is not JSON; {message} is the parser's explanation.",
      }),
    notObject: i18n._({
      id: "jsonEditor.notObject",
      message: "A document must be a JSON object.",
      comment: "Finding shown when the text is JSON but not an object.",
    }),
  };
}

export type JsonEditorMessages = ReturnType<typeof jsonEditorMessages>;
