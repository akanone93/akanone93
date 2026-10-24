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
  };
}
