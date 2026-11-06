// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** The words of the previewed locale's tools and the Localisation view (PC-32). */
export function localeToolsMessages(i18n: I18n) {
  return {
    title: (locale: string) =>
      i18n._({
        id: "localeTools.title",
        message: "In {locale}",
        values: { locale },
        comment: "Heading of the selected widget's texts in the previewed locale, e.g. In de-DE.",
      }),
    translated: i18n._({
      id: "localeTools.translated",
      message: "Translated in this locale",
      comment: "The text's key has a message in the previewed locale.",
    }),
    missing: (locale: string, fallback: string) =>
      i18n._({
        id: "localeTools.missing",
        message: "No message in {locale}: the device shows the {fallback} one",
        values: { locale, fallback },
        comment: "The text's key has no message in the previewed locale and falls back (I18N-006).",
      }),
    sample: i18n._({
      id: "localeTools.sample",
      message: "Plural sample",
      comment: "The count a plural message is shown with on the canvas.",
    }),
    fromData: i18n._({
      id: "localeTools.fromData",
      message: "From data",
      comment: "Option: the count comes from the page's data.",
    }),
    overrides: (locale: string) =>
      i18n._({
        id: "localeTools.overrides",
        message: "Only in {locale}",
        values: { locale },
        comment: "Heading of the props the widget sets differently in the previewed locale (I18N-012).",
      }),
    prop: i18n._({ id: "localeTools.prop", message: "Property", comment: "Select of the widget's props." }),
    value: i18n._({
      id: "localeTools.value",
      message: "Value in this locale",
      comment: "A prop's value in this locale, as JSON or text.",
    }),
    addOverride: (locale: string) =>
      i18n._({
        id: "localeTools.addOverride",
        message: "Set for {locale}",
        values: { locale },
        comment: "Button that sets the prop differently in the previewed locale.",
      }),
    removeOverride: (prop: string) =>
      i18n._({
        id: "localeTools.removeOverride",
        message: "Remove the {prop} override",
        values: { prop },
        comment: "Button.",
      }),
  };
}

export type LocaleToolsMessages = ReturnType<typeof localeToolsMessages>;
