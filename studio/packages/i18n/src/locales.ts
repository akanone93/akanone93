// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** A locale Studio can run in. The list is data: adding a locale adds an entry here and a catalogue. */
export interface LocaleDefinition {
  readonly tag: string;
  readonly direction: "ltr" | "rtl";
  /** Shown to users in the locale's own language, so it is data and not a catalogue message. */
  readonly nativeName: string;
}

/** Shipped locales; `en` is the source (STU-012). */
export const supportedLocales: readonly LocaleDefinition[] = [
  { tag: "en", direction: "ltr", nativeName: "English" },
  { tag: "de", direction: "ltr", nativeName: "Deutsch" },
  { tag: "ar", direction: "rtl", nativeName: "العربية" },
];

/** Pseudo-locales, offered in development and tests only (I18N-010). */
export const pseudoLocales: readonly LocaleDefinition[] = [
  { tag: "en-XA", direction: "ltr", nativeName: "English (accented)" },
  { tag: "ar-XB", direction: "rtl", nativeName: "Arabic (mirrored)" },
];

export const sourceLocale = "en";

/** The locales offered in this build; pseudo-locales only when `pseudo` is true. */
export function availableLocales(pseudo: boolean): readonly LocaleDefinition[] {
  return pseudo ? [...supportedLocales, ...pseudoLocales] : supportedLocales;
}
