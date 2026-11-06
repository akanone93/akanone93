// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of the selection's Theme card (PC-35). */
export function themeCardMessages(i18n: I18n) {
  return {
    title: i18n._({
      id: "themeCard.title",
      message: "Theme",
      comment: "Heading of the selected widget's colours and sizes, with their design tokens.",
    }),
    none: i18n._({
      id: "themeCard.none",
      message: "This widget sets no colours or sizes of its own.",
      comment: "Shown when the widget has no colour or size props set.",
    }),
    live: i18n._({
      id: "themeCard.live",
      message: "A token's value changes every widget that uses it, on every screen.",
      comment: "Hint under the Theme card heading.",
    }),
    usedBy: (n: number) =>
      i18n._({
        id: "themeCard.usedBy",
        message: "{n, plural, one {Used by # widget} other {Used by # widgets}}",
        values: { n },
        comment: "How many widgets on the plugin's screens use the token.",
      }),
    select: (token: string) =>
      i18n._({
        id: "themeCard.select",
        message: "Select the widgets that use {token}",
        values: { token },
        comment: "Button that selects every widget using the token on the canvas.",
      }),
    value: (token: string) =>
      i18n._({
        id: "themeCard.value",
        message: "Value of {token}",
        values: { token },
        comment: "Field that edits the token's value in the theme.",
      }),
    raw: i18n._({
      id: "themeCard.raw",
      message: "Not a token",
      comment: "Badge: the value is written in the widget, not taken from the theme.",
    }),
    bindTo: (token: string) =>
      i18n._({
        id: "themeCard.bindTo",
        message: "Use {token}",
        values: { token },
        comment: "Button that replaces the raw value with the theme's token of the same value.",
      }),
    makeToken: i18n._({
      id: "themeCard.makeToken",
      message: "Make a token",
      comment: "Button that adds the raw value to the theme as a new token and uses it.",
    }),
    tokenName: i18n._({
      id: "themeCard.tokenName",
      message: "Token name",
      comment: "Field: the new token's dotted name.",
    }),
    themeLocked: i18n._({
      id: "themeCard.themeLocked",
      message: "The theme is being edited elsewhere: its tokens are read-only here.",
      comment: "Shown when the app's theme cannot be edited from the canvas.",
    }),
  };
}

export type ThemeCardMessages = ReturnType<typeof themeCardMessages>;
