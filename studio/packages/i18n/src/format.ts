// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

export interface Formatters {
  number(value: number, options?: Intl.NumberFormatOptions): string;
  date(value: Date | number, options?: Intl.DateTimeFormatOptions): string;
  relativeTime(value: number, unit: Intl.RelativeTimeFormatUnit, options?: Intl.RelativeTimeFormatOptions): string;
  list(items: readonly string[], options?: Intl.ListFormatOptions): string;
}

/** Intl helpers bound to a locale tag (CLDR data, no hand-written plural or list rules). */
export function createFormatters(locale: string): Formatters {
  // The pseudo-locales are not CLDR tags; format them like the locale they imitate.
  const tag = locale === "en-XA" ? "en" : locale === "ar-XB" ? "ar" : locale;
  return {
    number: (value, options) => new Intl.NumberFormat(tag, options).format(value),
    date: (value, options) => new Intl.DateTimeFormat(tag, options).format(value),
    relativeTime: (value, unit, options) => new Intl.RelativeTimeFormat(tag, options).format(value, unit),
    list: (items, options) => new Intl.ListFormat(tag, options).format(items),
  };
}
