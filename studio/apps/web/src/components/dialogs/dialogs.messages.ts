// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages shared by the dialogs D3-D8 (STU-012). */
export function dialogMessages(i18n: I18n) {
  return {
    close: i18n._({ id: "dialog.close", message: "Close", comment: "Accessible name of a dialog's close button." }),
    cancel: i18n._({ id: "dialog.cancel", message: "Cancel", comment: "Button that dismisses a dialog without acting." }),
    confirm: i18n._({ id: "dialog.confirm", message: "Confirm", comment: "Button that confirms the action of a dialog." }),
    stepUpTitle: i18n._({
      id: "dialog.stepup.title",
      message: "Confirm it is you",
      comment: "Title of the dialog asking for a security key or passkey before a sensitive action.",
    }),
    stepUpBody: i18n._({
      id: "dialog.stepup.body",
      message: "This action needs a fresh check with your security key or passkey.",
      comment: "Explains why the step-up dialog appears.",
    }),
    stepUpUse: i18n._({
      id: "dialog.stepup.use",
      message: "Use security key or passkey",
      comment: "Button that starts the WebAuthn check.",
    }),
    stepUpFailed: i18n._({
      id: "dialog.stepup.failed",
      message: "The check did not succeed. Try again.",
      comment: "Shown when the WebAuthn check was cancelled or failed.",
    }),
    typeConfirmTitle: i18n._({
      id: "dialog.typeconfirm.title",
      message: "Confirm destructive action",
      comment: "Title of the type-to-confirm dialog.",
    }),
    typeConfirmPrompt: (phrase: string) =>
      i18n._({
      id: "dialog.typeconfirm.prompt",
      message: "Type {phrase} to confirm",
      comment: "Label of the field where the person retypes the phrase; {phrase} is the exact text to type.",
      values: { phrase },
    }),
    justificationTitle: i18n._({
      id: "dialog.justification.title",
      message: "Break-glass justification",
      comment: "Title of the dialog asking why emergency access is used.",
    }),
    justificationLabel: i18n._({
      id: "dialog.justification.label",
      message: "Justification",
      comment: "Label of the free-text reason field.",
    }),
    justificationHint: (count: number, min: number) =>
      i18n._({
      id: "dialog.justification.hint",
      message: "{count} of at least {min} characters",
      comment: "Progress under the justification field; {count} typed so far, {min} required.",
      values: { count, min },
    }),
    signedOutTitle: i18n._({
      id: "dialog.signedout.title",
      message: "You have been signed out",
      comment: "Title of the dialog shown when the session ended.",
    }),
    signedOutBody: i18n._({
      id: "dialog.signedout.body",
      message: "Your session ended. Sign in again to continue where you were; your work on this page is kept.",
      comment: "Explains the signed-out dialog.",
    }),
    signIn: i18n._({ id: "dialog.signedout.signin", message: "Sign in again", comment: "Button that opens the sign-in window." }),
    unsavedTitle: i18n._({
      id: "dialog.unsaved.title",
      message: "Leave without saving?",
      comment: "Title of the dialog shown when leaving a page with unsaved changes.",
    }),
    unsavedBody: i18n._({
      id: "dialog.unsaved.body",
      message: "You have changes that are not saved. If you leave now they are lost.",
      comment: "Explains the unsaved-changes dialog.",
    }),
    unsavedStay: i18n._({ id: "dialog.unsaved.stay", message: "Stay", comment: "Button that keeps the person on the page." }),
    unsavedLeave: i18n._({ id: "dialog.unsaved.leave", message: "Leave", comment: "Button that leaves and discards changes." }),
    explainTitle: i18n._({
      id: "dialog.explain.title",
      message: "Why is this unavailable?",
      comment: "Title of the dialog that explains an access decision.",
    }),
    explainDecision: (decision: string) =>
      i18n._({
      id: "dialog.explain.decision",
      message: "Decision: {decision}",
      comment: "Shows the access decision; {decision} is the server's word.",
      values: { decision },
    }),
    explainFailed: i18n._({
      id: "dialog.explain.failed",
      message: "The explanation could not be loaded.",
      comment: "Shown when explain-access fails.",
    }),
    explainLoading: i18n._({ id: "dialog.explain.loading", message: "Checking access", comment: "Shown while the explanation loads." }),
  };
}
