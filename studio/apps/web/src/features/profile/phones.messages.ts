// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of the person's phones signed in to the Plux Dev app (DEV-002, ADR-0104). */
export function phonesMessages(i18n: I18n) {
  return {
    title: i18n._({
      id: "profile.phones.title",
      message: "Phones",
      comment: "Title of the card listing the phones signed in to the Plux Dev app.",
    }),
    caption: i18n._({
      id: "profile.phones.caption",
      message:
        "Phones signed in to the Plux Dev app. A phone lists your apps and joins the session you have open in Studio once you confirm a number. A phone unused for 30 days is signed out.",
      comment: "Description of the phones card.",
    }),
    empty: i18n._({
      id: "profile.phones.empty",
      message: "No phone is signed in yet.",
      comment: "Shown in the phones card when no phone is signed in.",
    }),
    connect: i18n._({
      id: "profile.phones.connect",
      message: "Connect a phone",
      comment: "Button that starts connecting a phone to the Plux Dev app; it asks for the security key first.",
    }),
    connectFailed: i18n._({
      id: "profile.phones.connectFailed",
      message: "The phone could not be connected. Confirm with your security key and try again.",
      comment: "Shown when starting to connect a phone failed, usually because the security key step was cancelled.",
    }),
    lastUsed: (when: string) =>
      i18n._({
        id: "profile.phones.lastUsed",
        message: "Last used {when}",
        values: { when },
        comment: "When a phone was last used; when is a localised date and time.",
      }),
    neverUsed: i18n._({
      id: "profile.phones.neverUsed",
      message: "Not used yet",
      comment: "Shown for a phone that has not been used since it was signed in.",
    }),
    signOut: (name: string) =>
      i18n._({
        id: "profile.phones.signOut",
        message: "Sign out {name}",
        values: { name },
        comment: "Accessible name of the button that signs a phone out; name is the phone's name.",
      }),
    signOutTitle: i18n._({
      id: "profile.phones.signOutTitle",
      message: "Sign this phone out?",
      comment: "Title of the confirmation before a phone is signed out.",
    }),
    signOutBody: i18n._({
      id: "profile.phones.signOutBody",
      message: "The Plux Dev app on it can no longer open your apps. You can connect it again at any time.",
      comment: "Body of the confirmation before a phone is signed out.",
    }),
    signOutConfirm: i18n._({
      id: "profile.phones.signOutConfirm",
      message: "Sign out",
      comment: "Confirms signing a phone out.",
    }),
    cancel: i18n._({
      id: "profile.phones.cancel",
      message: "Cancel",
      comment: "Closes a phone dialog without changing anything.",
    }),
    close: i18n._({
      id: "profile.phones.close",
      message: "Close",
      comment: "Closes the dialog that connects a phone.",
    }),
    dialogTitle: i18n._({
      id: "profile.phones.dialogTitle",
      message: "Connect a phone",
      comment: "Title of the dialog that shows the QR code to connect a phone.",
    }),
    scan: i18n._({
      id: "profile.phones.scan",
      message: "Open the Plux Dev app on the phone and scan this code.",
      comment: "Instruction under the QR code that connects a phone.",
    }),
    qrLabel: i18n._({
      id: "profile.phones.qrLabel",
      message: "QR code that connects a phone to the Plux Dev app",
      comment: "Accessible name of the QR code that connects a phone.",
    }),
    expiresIn: (seconds: number) =>
      i18n._({
        id: "profile.phones.expiresIn",
        message: "{seconds, plural, one {Works once, for # more second} other {Works once, for # more seconds}}",
        values: { seconds },
        comment: "How long the QR code that connects a phone stays valid.",
      }),
    pickOn: (phone: string) =>
      i18n._({
        id: "profile.phones.pickOn",
        message: "Pick this number on {phone}",
        values: { phone },
        comment: "Shown above the number to pick, once a phone scanned the code; phone is its name.",
      }),
    number: (n: number) =>
      i18n._({
        id: "profile.phones.number",
        message: "Number to pick: {n}",
        values: { n },
        comment: "Accessible name of the large two-digit number to pick on the phone.",
      }),
    pickHint: i18n._({
      id: "profile.phones.pickHint",
      message: "Someone who copied the code cannot finish without this number.",
      comment: "Why the number is asked for, under the number to pick.",
    }),
    connected: (phone: string) =>
      i18n._({
        id: "profile.phones.connected",
        message: "{phone} is connected. Tap an app on it to open it.",
        values: { phone },
        comment: "Shown once a phone is signed in; phone is its name.",
      }),
    wrong: i18n._({
      id: "profile.phones.wrong",
      message: "A wrong number was picked, so the phone was not connected. Show a new code to try again.",
      comment: "Shown when the number picked on the phone was not the one shown.",
    }),
    expired: i18n._({
      id: "profile.phones.expired",
      message: "The code expired. Show a new one to try again.",
      comment: "Shown when the QR code that connects a phone was not used in time.",
    }),
    again: i18n._({
      id: "profile.phones.again",
      message: "Show a new code",
      comment: "Button that starts connecting a phone again.",
    }),
  };
}
