// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** What a page says when the server refuses to show it, and how the person gets past the refusal. */
export function refusalMessages(i18n: I18n) {
  return {
    stepUpTitle: i18n._({
      id: "refusal.step-up.title",
      message: "Confirm it's you",
      comment: "Title shown when a page needs a fresh passkey confirmation before the server shows it.",
    }),
    stepUpBody: i18n._({
      id: "refusal.step-up.body",
      message: "This page shows sensitive settings. Confirm with your passkey to open it for the next five minutes.",
      comment: "Explains the step-up a page needs (SEC-103).",
    }),
    stepUpAction: i18n._({
      id: "refusal.step-up.action",
      message: "Confirm with passkey",
      comment: "Button that starts the passkey confirmation of a page.",
    }),
    stepUpFailed: i18n._({
      id: "refusal.step-up.failed",
      message: "The confirmation did not complete. Try again.",
      comment: "Shown when the passkey confirmation of a page failed or was cancelled.",
    }),
    secondFactorTitle: i18n._({
      id: "refusal.second-factor.title",
      message: "A second factor is required",
      comment: "Title shown when a page needs a session signed in with a second factor.",
    }),
    secondFactorBody: i18n._({
      id: "refusal.second-factor.body",
      message:
        "This session was signed in with a password only. Sign out, then sign in with a passkey or your second factor to open this page.",
      comment: "Explains how to get a session with a second factor.",
    }),
    licenceTitle: i18n._({
      id: "refusal.licence.title",
      message: "Not included in your licence",
      comment: "Title shown when the organisation's licence does not include the feature of a page.",
    }),
    licenceBody: i18n._({
      id: "refusal.licence.body",
      message: "This feature is part of the enterprise edition. Add a licence that includes it to use this page.",
      comment: "Explains a page whose feature the licence does not include.",
    }),
    licenceAction: i18n._({
      id: "refusal.licence.action",
      message: "View licence",
      comment: "Button that opens the organisation's licence page.",
    }),
    permissionTitle: i18n._({
      id: "refusal.permission.title",
      message: "You don't have access",
      comment: "Title shown when the server refuses a page for a missing permission.",
    }),
    permissionBody: i18n._({
      id: "refusal.permission.body",
      message: "Ask an administrator of the organisation for the permission this page needs.",
      comment: "Explains a page refused for a missing permission.",
    }),
    failedTitle: i18n._({
      id: "refusal.failed.title",
      message: "This could not be loaded",
      comment: "Title shown when a page's data could not be loaded for another reason.",
    }),
    failedBody: i18n._({
      id: "refusal.failed.body",
      message: "The server did not answer as expected. Try again in a moment.",
      comment: "Explains a page whose data could not be loaded.",
    }),
    retry: i18n._({
      id: "refusal.retry",
      message: "Try again",
      comment: "Button that loads a page's data again.",
    }),
    code: (code: string) =>
      i18n._({
        id: "refusal.code",
        message: "Error {code}",
        values: { code },
        comment: "The stable error code of a refusal, for support (for example PLX-6200).",
      }),
  };
}
