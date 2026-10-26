// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the safety rules: type-to-confirm, blocked deletes and rename warnings (decisions F13). */
export function safetyMessages(i18n: I18n) {
  return {
    confirmInstruction: (name: string) =>
      i18n._({
        id: "safety.confirm.instruction",
        message: "Type {name} to confirm",
        values: { name },
        comment: "STU-074 type-to-confirm: label of the field where the user types the name of what is affected.",
      }),
    confirmMismatch: i18n._({
      id: "safety.confirm.mismatch",
      message: "The text does not match yet",
      comment: "STU-074 type-to-confirm: shown while the typed text differs from the name.",
    }),
    cancel: i18n._({
      id: "safety.cancel",
      message: "Cancel",
      comment: "Button that closes a safety dialog without doing anything.",
    }),
    close: i18n._({
      id: "safety.close",
      message: "Close",
      comment: "Accessible name of a safety dialog's close button.",
    }),
    blockedTitle: i18n._({
      id: "safety.blocked.title",
      message: "This cannot be deleted yet",
      comment: "D19, D24, D26: title of the panel listing what still uses or links to the item being deleted.",
    }),
    blockedEntry: i18n._({
      id: "safety.blocked.entry",
      message: "It is an entry point. Choose another entry first.",
      comment: "STU-071: the item is the app's or plugin's entry and cannot be deleted until another is chosen.",
    }),
    blockedIncoming: (count: number) =>
      i18n._({
        id: "safety.blocked.incoming",
        message: "{count, plural, one {# use or link into it} other {# uses or links into it}}",
        values: { count },
        comment: "STU-070, STU-072: heading of the list of uses of, or links into, the item.",
      }),
    blockedOutgoing: (count: number) =>
      i18n._({
        id: "safety.blocked.outgoing",
        message: "{count, plural, one {# link leaves it} other {# links leave it}}",
        values: { count },
        comment: "STU-070: heading of the list of links from a plugin to other plugins.",
      }),
    redirectIncoming: i18n._({
      id: "safety.fix.redirect",
      message: "Redirect all incoming links to…",
      comment: "STU-070 assisted fix: send every link into this screen to another screen instead.",
    }),
    removeOutgoing: i18n._({
      id: "safety.fix.remove-outgoing",
      message: "Remove outgoing links",
      comment: "STU-070 assisted fix: delete every navigation step that leaves this plugin.",
    }),
    goToUse: (title: string) =>
      i18n._({
        id: "safety.use.go",
        message: "Go to {title}",
        values: { title },
        comment: "Accessible name of the button that navigates to one use of the item.",
      }),
    renameTitle: i18n._({
      id: "safety.rename.title",
      message: "This change affects code outside Studio",
      comment: "D27, STU-076: title of the warning listing deep links and generated APIs a key or route change breaks.",
    }),
    renameDeepLink: i18n._({
      id: "safety.rename.deep-link",
      message: "Deep link",
      comment: "D27: type of an affected item, a URL path that opens the screen.",
    }),
    renameGenerated: i18n._({
      id: "safety.rename.generated",
      message: "Generated API",
      comment: "D27: type of an affected item, a member of the Dart API that plux codegen writes for host apps.",
    }),
  };
}
