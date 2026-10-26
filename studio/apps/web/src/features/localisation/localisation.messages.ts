// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the localisation table and of key extraction (S5.5, STU-063, I18N-007). */
export function localisationMessages(i18n: I18n) {
  return {
    open: i18n._({
      id: "l10n.open",
      message: "Localisation table",
      comment: "Button in the right panel that opens the full-screen table of translation keys and locales.",
    }),
    title: i18n._({
      id: "l10n.title",
      message: "Localisation",
      comment: "Title of the full-screen overlay in which translation keys are edited for every locale.",
    }),
    close: i18n._({
      id: "l10n.close",
      message: "Close the localisation table",
      comment: "Accessible name of the button that closes the localisation table.",
    }),
    scope: i18n._({
      id: "l10n.scope",
      message: "Keys shown",
      comment: "Label of the list that chooses which translation keys the table shows.",
    }),
    scopePlugin: i18n._({
      id: "l10n.scope.plugin",
      message: "Used by this plugin",
      comment: "Option: only the translation keys that the pages of this plugin use.",
    }),
    scopeSelection: i18n._({
      id: "l10n.scope.selection",
      message: "Used by the selection",
      comment: "Option: only the translation keys that the selected nodes use.",
    }),
    scopeAll: i18n._({
      id: "l10n.scope.all",
      message: "All keys of the app",
      comment: "Option: every translation key of the app.",
    }),
    editLocale: i18n._({
      id: "l10n.editLocale",
      message: "Locale to edit",
      comment:
        "Label of the list that chooses the locale whose messages the table edits; each locale has its own lock.",
    }),
    table: i18n._({
      id: "l10n.table",
      message: "Translation keys by locale",
      comment: "Accessible name of the table of translation keys and their messages in every locale.",
    }),
    keyColumn: i18n._({
      id: "l10n.column.key",
      message: "Key",
      comment: "Column heading of the table: the name of the translation key.",
    }),
    messageOf: (key: string, locale: string) =>
      i18n._({
        id: "l10n.message",
        message: "{key} in {locale}",
        values: { key, locale },
        comment: "Accessible name of the field that holds one message: the key and the locale it is in.",
      }),
    missing: i18n._({
      id: "l10n.missing",
      message: "Missing",
      comment: "Shown in a cell of the table whose locale has no message for the key yet.",
    }),
    empty: i18n._({
      id: "l10n.empty",
      message: "No translation keys to show.",
      comment: "Shown when the table has no key for the chosen scope.",
    }),
    readOnly: i18n._({
      id: "l10n.readOnly",
      message: "Another person holds the lock for this locale, so the table is read-only.",
      comment: "Notice when the lock of the locale being edited is held by someone else.",
    }),
    newKey: i18n._({
      id: "l10n.newKey",
      message: "New key",
      comment: "Label of the field in which the name of a new translation key is typed.",
    }),
    addKey: i18n._({
      id: "l10n.addKey",
      message: "Add key",
      comment: "Button that adds the typed translation key to the app.",
    }),
    keysReadOnly: i18n._({
      id: "l10n.keysReadOnly",
      message: "Keys cannot be added while another person holds the draft lock.",
      comment: "Notice when the app's draft lock, which new keys need, is held by someone else.",
    }),
    problem: (kind: "empty" | "invalid" | "taken") =>
      kind === "empty"
        ? i18n._({
            id: "l10n.problem.empty",
            message: "Enter a key name.",
            comment: "Error under the key name field when it is empty.",
          })
        : kind === "taken"
          ? i18n._({
              id: "l10n.problem.taken",
              message: "A key with this name exists already.",
              comment: "Error under the key name field when the name is in use.",
            })
          : i18n._({
              id: "l10n.problem.invalid",
              message: "Use lower-case words joined by dots or hyphens, such as home.title.",
              comment: "Error under the key name field when the name does not follow the key pattern.",
            }),
    extract: i18n._({
      id: "l10n.extract",
      message: "Move to a translation key",
      comment: "Button under a text property: offers to turn the typed text into a translation key.",
    }),
    extractTitle: i18n._({
      id: "l10n.extract.title",
      message: "Move the text to a translation key",
      comment: "Title of the dialog that creates a translation key from literal text.",
    }),
    extractHint: (locale: string) =>
      i18n._({
        id: "l10n.extract.hint",
        message:
          "The text becomes the {locale} message of the key and the property refers to the key. This is one undo step.",
        values: { locale },
        comment: "Explains what moving literal text to a translation key does; the locale is the app's default.",
      }),
    extractText: i18n._({
      id: "l10n.extract.text",
      message: "Text",
      comment: "Label of the read-only field that shows the text being moved to a key.",
    }),
    extractConfirm: i18n._({
      id: "l10n.extract.confirm",
      message: "Create key",
      comment: "Button that creates the translation key and replaces the text with it.",
    }),
    extractCancel: i18n._({
      id: "l10n.extract.cancel",
      message: "Cancel",
      comment: "Button that closes the key extraction dialog without changes.",
    }),
    extractFailed: i18n._({
      id: "l10n.extract.failed",
      message: "The key could not be saved. Nothing was changed.",
      comment: "Error in the key extraction dialog when the server did not accept the new key or message.",
    }),
    extractUnavailable: i18n._({
      id: "l10n.extract.unavailable",
      message: "Keys can be created once the draft lock and the default locale's lock are yours.",
      comment: "Notice in the key extraction dialog when a lock the extraction needs is held by someone else.",
    }),
  };
}

export type LocalisationMessages = ReturnType<typeof localisationMessages>;
