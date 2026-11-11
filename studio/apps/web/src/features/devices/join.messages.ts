// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of a phone's request to join the dev session (DEV-002, ADR-0104). */
export function joinMessages(i18n: I18n) {
  return {
    asks: (phone: string) =>
      i18n._({
        id: "devices.join.asks",
        message: "{phone} wants to join",
        values: { phone },
        comment: "Title of the prompt shown when a signed-in phone asks to join the dev session; phone is its name.",
      }),
    pick: i18n._({
      id: "devices.join.pick",
      message: "Pick this number on the phone to let it in. Only you see it here.",
      comment: "Under the two-digit number a person picks on their phone to let it join the dev session.",
    }),
    number: (n: number) =>
      i18n._({
        id: "devices.join.number",
        message: "Number to pick: {n}",
        values: { n },
        comment: "Accessible name of the large two-digit number of a join request.",
      }),
    deny: i18n._({
      id: "devices.join.deny",
      message: "Deny",
      comment: "Button that refuses a phone's request to join the dev session.",
    }),
    expires: (seconds: number) =>
      i18n._({
        id: "devices.join.expires",
        message: "{seconds, plural, one {# second left} other {# seconds left}}",
        values: { seconds },
        comment: "How long a phone's request to join stays open.",
      }),
    wrong: (phone: string) =>
      i18n._({
        id: "devices.join.wrong",
        message: "A wrong number was picked on {phone}",
        values: { phone },
        comment: "Title shown when the number picked on a phone was not the one Studio showed; phone is its name.",
      }),
    wrongBody: i18n._({
      id: "devices.join.wrongBody",
      message:
        "The phone can't join this session until you let it try again. If it wasn't you, sign the phone out under Profile › Phones.",
      comment: "Body of the prompt after a wrong number was picked on a phone.",
    }),
    retry: i18n._({
      id: "devices.join.retry",
      message: "Let it try again",
      comment: "Button that lets a phone that picked a wrong number ask to join again.",
    }),
    dismiss: i18n._({
      id: "devices.join.dismiss",
      message: "Dismiss",
      comment: "Button that closes the prompt about a wrong number, leaving the phone out of the session.",
    }),
    failed: i18n._({
      id: "devices.join.failed",
      message: "That did not work. Try again.",
      comment: "Shown on a join prompt when denying or letting the phone try again failed.",
    }),
  };
}
