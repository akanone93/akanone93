// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type I18n, type Messages, setupI18n } from "@lingui/core";
import { directionOf } from "./direction.ts";
import { createFormatters, type Formatters } from "./format.ts";
import { availableLocales, sourceLocale } from "./locales.ts";
import { resolveLocale } from "./negotiate.ts";
import { pseudoCatalogue } from "./pseudo.ts";

/** Loads a compiled catalogue; messages may be compiled functions or plain strings. */
export type CatalogueLoader = (locale: string) => Promise<Messages>;

/** Called for each message missing in the active locale (development and tests, ADR-0094). */
export type MissingReporter = (locale: string, id: string) => void;

export interface StudioI18nOptions {
  readonly load: CatalogueLoader;
  readonly pseudo?: boolean;
  readonly onMissing?: MissingReporter;
  /** The document whose `lang` and `dir` follow the locale. */
  readonly root?: HTMLElement;
}

export interface StudioI18n {
  readonly i18n: I18n;
  readonly locale: () => string;
  readonly formatters: () => Formatters;
  /** Loads the catalogue (once per locale) and activates it without a reload. */
  readonly activate: (requested: string) => Promise<string>;
}

/** Creates an instance; the app owns the only one, there is no module-level singleton. */
export function createStudioI18n(options: StudioI18nOptions): StudioI18n {
  const pseudo = options.pseudo ?? false;
  const i18n = setupI18n({
    missing: (locale, id) => {
      options.onMissing?.(locale, id);
      // A key is never shown: fall back to the source text, which the en catalogue holds.
      const source = i18n.messages;
      return typeof source[id] === "string" ? source[id] : "";
    },
  });
  const loaded = new Set<string>();
  const sourceMessages = async (): Promise<Messages> => options.load(sourceLocale);

  const ensure = async (locale: string): Promise<void> => {
    if (loaded.has(locale)) {
      return;
    }
    let messages: Messages;
    if (locale === "en-XA" || locale === "ar-XB") {
      const source = await sourceMessages();
      messages = pseudoCatalogue(locale, source);
    } else {
      messages = await options.load(locale);
    }
    const english = locale === sourceLocale ? messages : await sourceMessages();
    // Missing ids fall back to English text, and are reported.
    const merged: Messages = { ...english, ...messages };
    for (const id of Object.keys(english)) {
      if (!(id in messages)) {
        options.onMissing?.(locale, id);
      }
    }
    i18n.load(locale, merged);
    loaded.add(locale);
  };

  return {
    i18n,
    locale: () => i18n.locale,
    formatters: () => createFormatters(i18n.locale),
    activate: async (requested) => {
      const locale = resolveLocale(requested, pseudo);
      await ensure(locale);
      i18n.activate(locale);
      const root = options.root ?? (typeof document === "undefined" ? undefined : document.documentElement);
      if (root !== undefined) {
        root.lang = availableLocales(pseudo).some((l) => l.tag === locale) ? locale : sourceLocale;
        root.dir = directionOf(locale);
      }
      return locale;
    },
  };
}
