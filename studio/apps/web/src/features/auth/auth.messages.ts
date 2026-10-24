// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the sign-in and invitation pages (STU-012). */
export function authMessages(i18n: I18n) {
  return {
    signInTitle: i18n._({ id: "auth.signin.title", message: "Sign in", comment: "Heading of the sign-in page." }),
    email: i18n._({ id: "auth.field.email", message: "Email", comment: "Label of the email field." }),
    password: i18n._({ id: "auth.field.password", message: "Password", comment: "Label of the password field." }),
    submit: i18n._({ id: "auth.signin.submit", message: "Sign in", comment: "Button that submits the password." }),
    submitting: i18n._({
      id: "auth.signin.submitting",
      message: "Signing in",
      comment: "Button label while the sign-in request runs.",
    }),
    emailInvalid: i18n._({
      id: "auth.error.email",
      message: "Enter a valid email address.",
      comment: "Validation error of the email field.",
    }),
    passwordRequired: i18n._({
      id: "auth.error.password",
      message: "Enter your password.",
      comment: "Validation error of the password field.",
    }),
    rejected: i18n._({
      id: "auth.error.rejected",
      message: "The email or password is not correct.",
      comment: "Shown when the server refuses the credentials.",
    }),
    failed: i18n._({
      id: "auth.error.failed",
      message: "Sign-in failed. Try again in a moment.",
      comment: "Shown when the sign-in request fails for another reason.",
    }),
    factorTitle: i18n._({
      id: "auth.factor.title",
      message: "Second factor",
      comment: "Heading of the second-factor step.",
    }),
    code: i18n._({
      id: "auth.factor.code",
      message: "Authentication code",
      comment: "Label of the field for the one-time code.",
    }),
    codeInvalid: i18n._({
      id: "auth.error.code",
      message: "Enter the 6-digit code.",
      comment: "Validation error of the one-time code.",
    }),
    codeRejected: i18n._({
      id: "auth.error.code-rejected",
      message: "The code is not correct or has expired.",
      comment: "Shown when the server refuses the one-time code.",
    }),
    verify: i18n._({ id: "auth.factor.verify", message: "Verify", comment: "Button that submits the one-time code." }),
    passkey: i18n._({
      id: "auth.factor.passkey",
      message: "Use a passkey",
      comment: "Button that answers the second factor with a passkey.",
    }),
    passkeyFailed: i18n._({
      id: "auth.error.passkey",
      message: "The passkey could not be used.",
      comment: "Shown when the passkey prompt fails or is cancelled.",
    }),
    providers: i18n._({
      id: "auth.providers",
      message: "Single sign-on",
      comment: "Accessible name of the group of single sign-on buttons.",
    }),
    continueWith: (name: string) =>
      i18n._({
        id: "auth.provider.continue",
        message: "Continue with {name}",
        values: { name },
        comment: "Button that starts single sign-on; name is the identity provider.",
      }),
    inviteTitle: i18n._({
      id: "auth.invite.title",
      message: "Accept your invitation",
      comment: "Heading of the invitation page.",
    }),
    displayName: i18n._({
      id: "auth.invite.name",
      message: "Your name",
      comment: "Label of the display-name field on the invitation page.",
    }),
    nameRequired: i18n._({
      id: "auth.error.name",
      message: "Enter your name.",
      comment: "Validation error of the display-name field.",
    }),
    newPassword: i18n._({
      id: "auth.invite.password",
      message: "Choose a password",
      comment: "Label of the new-password field on the invitation page.",
    }),
    passwordShort: i18n._({
      id: "auth.error.password-short",
      message: "Use at least 12 characters.",
      comment: "Validation error of the new password.",
    }),
    accept: i18n._({
      id: "auth.invite.accept",
      message: "Accept invitation",
      comment: "Button that accepts the invitation.",
    }),
    inviteFailed: i18n._({
      id: "auth.error.invite",
      message: "The invitation is invalid or has expired.",
      comment: "Shown when the invitation is refused.",
    }),
  };
}
