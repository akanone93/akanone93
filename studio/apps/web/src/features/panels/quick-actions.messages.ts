// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of the quick actions and the handler list (PC-27, ACT-001–ACT-003). */
export function quickActionMessages(i18n: I18n) {
  return {
    title: i18n._({
      id: "quick.title",
      message: "Quick actions",
      comment: "Heading of the buttons that give an event a common action in one step.",
    }),
    event: i18n._({
      id: "quick.event",
      message: "When",
      comment: "Label of the event a quick action runs on, e.g. onPressed.",
    }),
    kind: (kind: string) =>
      i18n._({
        id: "quick.kind",
        message:
          "{kind, select, page {Go to page} dialog {Open a dialog} sheet {Open a sheet} message {Show a message} link {Open a link} back {Go back} state {Set state} submit {Submit a form} other {Action}}",
        values: { kind },
        comment: "A quick action an event can run.",
      }),
    field: (key: string) =>
      i18n._({
        id: "quick.field",
        message:
          "{key, select, route {Page} message {Message} actionLabel {Button (optional)} url {Web address} path {State path} value {Value} form {Form} other {Value}}",
        values: { key },
        comment: "A field of a quick action, e.g. the message a snackbar shows.",
      }),
    replace: i18n._({
      id: "quick.replace",
      message: "Replace what it runs",
      comment: "Option: the new action replaces the event's steps.",
    }),
    append: i18n._({
      id: "quick.append",
      message: "Run after what it runs",
      comment: "Option: the new action runs after the event's steps.",
    }),
    add: i18n._({ id: "quick.add", message: "Add action", comment: "Button that writes the quick action." }),
    handlers: i18n._({
      id: "quick.handlers",
      message: "What its events run",
      comment: "Heading of the list of the selected widget's event steps.",
    }),
    none: i18n._({
      id: "quick.none",
      message: "Its events run nothing yet.",
      comment: "Shown when the widget's events have no steps.",
    }),
    removeStep: (action: string) =>
      i18n._({
        id: "quick.removeStep",
        message: "Remove {action}",
        values: { action },
        comment: "Button that removes one step of an event.",
      }),
    noPages: (kind: string) =>
      i18n._({
        id: "quick.noPages",
        message:
          "{kind, select, dialog {No dialog page yet: add one with New screen › Dialog.} other {No bottom sheet page yet: add one with New screen › Bottom sheet.}}",
        values: { kind },
        comment: "Shown when the app has no dialog or sheet page to open.",
      }),
    snackShown: i18n._({
      id: "quick.snackShown",
      message: "Its message shows at the bottom of the screen while it is selected.",
      comment: "Note under a widget whose event shows a snackbar.",
    }),
  };
}

export type QuickActionMessages = ReturnType<typeof quickActionMessages>;
