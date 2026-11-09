// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the profile page (STU-014, STU-012). */
export function profileMessages(i18n: I18n) {
  return {
    title: i18n._({ id: "profile.title", message: "Profile", comment: "Heading of the profile page." }),
    caption: i18n._({
      id: "profile.caption",
      message: "Your appearance and language, kept for every device.",
      comment: "Caption under the profile heading.",
    }),
    theme: i18n._({ id: "profile.theme", message: "Theme", comment: "Label of the theme choice." }),
    themeSystem: i18n._({
      id: "profile.theme.system",
      message: "Match system",
      comment: "Theme option that follows the operating system.",
    }),
    themeLight: i18n._({ id: "profile.theme.light", message: "Light", comment: "Theme option." }),
    themeDark: i18n._({ id: "profile.theme.dark", message: "Dark", comment: "Theme option." }),
    themeContrast: i18n._({ id: "profile.theme.contrast", message: "High contrast", comment: "Theme option." }),
    language: i18n._({ id: "profile.language", message: "Language", comment: "Label of the language choice." }),
    density: i18n._({ id: "profile.density", message: "Density", comment: "Label of the density choice." }),
    densityComfortable: i18n._({
      id: "profile.density.comfortable",
      message: "Comfortable",
      comment: "Density option.",
    }),
    densityCompact: i18n._({ id: "profile.density.compact", message: "Compact", comment: "Density option." }),
    saved: i18n._({ id: "profile.saved", message: "Saved", comment: "Confirmation after a preference was stored." }),
    saveFailed: i18n._({
      id: "profile.save-failed",
      message: "Your preference could not be saved.",
      comment: "Shown when storing a preference fails.",
    }),
    account: i18n._({
      id: "profile.account",
      message: "Account",
      comment: "Heading of the section with the person's name and email.",
    }),
    accountCaption: i18n._({
      id: "profile.account.caption",
      message: "The name and email address you sign in with.",
      comment: "Caption of the account card on the profile page.",
    }),
    installationAdmin: i18n._({
      id: "profile.account.installation-admin",
      message: "Installation administrator",
      comment: "Badge on the profile of a person who administers the whole installation.",
    }),
    memberOf: (count: number) =>
      i18n._({
        id: "profile.account.member-of",
        message: "{count, plural, one {Member of # organisation} other {Member of # organisations}}",
        comment: "Line on the account card: how many organisations the person belongs to.",
        values: { count },
      }),
    appearance: i18n._({
      id: "profile.appearance",
      message: "Appearance",
      comment: "Heading of the card with the theme, density and language.",
    }),
    appearanceCaption: i18n._({
      id: "profile.appearance.caption",
      message: "How Studio looks and reads. A change applies at once.",
      comment: "Caption of the appearance card on the profile page.",
    }),
    themeCaption: i18n._({
      id: "profile.theme.caption",
      message: "Match system follows the light or dark setting of your device.",
      comment: "Description under the theme choice.",
    }),
    densityCaption: i18n._({
      id: "profile.density.caption",
      message: "Compact fits more rows on the screen.",
      comment: "Description under the density choice.",
    }),
    navProfile: i18n._({
      id: "profile.nav.profile",
      message: "Profile",
      comment: "Tab of the profile page with the account, appearance and notifications.",
    }),
    navShortcuts: i18n._({
      id: "profile.nav.shortcuts",
      message: "Keyboard shortcuts",
      comment: "Tab of the profile page with the keyboard shortcuts.",
    }),
    navLabel: i18n._({
      id: "profile.nav.label",
      message: "Profile pages",
      comment: "Accessible name of the tabs of the profile page.",
    }),
    keysTitle: i18n._({
      id: "profile.keys.title",
      message: "Security keys",
      comment: "Heading of the card that lists the person's security keys and passkeys.",
    }),
    keysCaption: i18n._({
      id: "profile.keys.caption",
      message:
        "A security key or passkey is your second factor: you sign in with it, and confirm publishing, approvals and command-line sign-ins with it.",
      comment: "Caption of the security keys card.",
    }),
    keysEmpty: i18n._({
      id: "profile.keys.empty",
      message: "No security key yet. Add one to publish, approve and sign in to the command line.",
      comment: "Shown when the person has no second factor.",
    }),
    keyLabel: i18n._({
      id: "profile.keys.label",
      message: "Name of the new key",
      comment: "Label of the field that names a security key being added, such as 'Laptop' or 'YubiKey'.",
    }),
    keyAdd: i18n._({ id: "profile.keys.add", message: "Add security key", comment: "Button that registers a key." }),
    keyAdding: i18n._({
      id: "profile.keys.adding",
      message: "Follow your browser's prompt",
      comment: "Button label while the browser asks for the security key.",
    }),
    keyAdded: i18n._({
      id: "profile.keys.added",
      message: "Security key added. This session now counts as signed in with it.",
      comment: "Confirmation after a key was registered.",
    }),
    keyKindWebAuthn: i18n._({ id: "profile.keys.kind.webauthn", message: "Security key", comment: "Kind of factor." }),
    keyKindTotp: i18n._({ id: "profile.keys.kind.totp", message: "Authenticator app", comment: "Kind of factor." }),
    keyUnnamed: i18n._({
      id: "profile.keys.unnamed",
      message: "Unnamed key",
      comment: "Shown for a security key registered without a name.",
    }),
    keyPending: i18n._({
      id: "profile.keys.pending",
      message: "Not finished",
      comment: "Badge of a key whose registration was started but not completed; it cannot be used.",
    }),
    keyLastUsed: (date: string) =>
      i18n._({
        id: "profile.keys.last-used",
        message: "Last used {date}",
        values: { date },
        comment: "When a key was last used; date is already formatted.",
      }),
    keyRemove: (name: string) =>
      i18n._({
        id: "profile.keys.remove",
        message: "Remove {name}",
        values: { name },
        comment: "Accessible label of the button that removes one key.",
      }),
    keyRemoveTitle: i18n._({
      id: "profile.keys.remove.title",
      message: "Remove this security key?",
      comment: "Title of the confirmation before removing a key.",
    }),
    keyRemoveBody: i18n._({
      id: "profile.keys.remove.body",
      message: "You can no longer sign in or confirm actions with it. Keep at least one key you can use.",
      comment: "Body of the confirmation before removing a key.",
    }),
    keyRemoveConfirm: i18n._({ id: "profile.keys.remove.confirm", message: "Remove", comment: "Confirm button." }),
    keyCancel: i18n._({ id: "profile.keys.cancel", message: "Cancel", comment: "Cancel button." }),
    keyUnsupported: i18n._({
      id: "profile.keys.unsupported",
      message: "This browser cannot use security keys or passkeys.",
      comment: "Shown when the browser has no WebAuthn support.",
    }),
    keyNeedsFactor: i18n._({
      id: "profile.keys.needs-factor",
      message: "Sign out and sign in again with one of your keys, then try again.",
      comment: "Shown when adding or removing a key needs a second factor presented in this session.",
    }),
    keyFailed: i18n._({
      id: "profile.keys.failed",
      message: "The security key was not added. Try again, or use another key.",
      comment: "Shown when a registration was cancelled or did not verify.",
    }),
    keyRemoveFailed: i18n._({
      id: "profile.keys.remove-failed",
      message: "The security key was not removed.",
      comment: "Shown when removing a key fails for another reason.",
    }),
  };
}
