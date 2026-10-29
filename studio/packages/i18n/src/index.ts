// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

export { type Direction, directionOf } from "./direction.ts";
export { createFormatters, type Formatters } from "./format.ts";
export {
  type CatalogueLoader,
  createStudioI18n,
  type MissingReporter,
  type StudioI18n,
  type StudioI18nOptions,
} from "./i18n.ts";
export { availableLocales, type LocaleDefinition, pseudoLocales, sourceLocale, supportedLocales } from "./locales.ts";
export { type NegotiationInput, negotiateLocale, parseAcceptLanguage, resolveLocale } from "./negotiate.ts";
export { pseudoAccent, pseudoCatalogue, pseudoMirror } from "./pseudo.ts";
