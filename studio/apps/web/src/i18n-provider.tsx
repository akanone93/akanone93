// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n, Messages } from "@lingui/core";
import { I18nProvider as LinguiProvider } from "@lingui/react";
import { createStudioI18n, directionOf, negotiateLocale, type StudioI18n } from "@plux/i18n";
import { createContext, type ReactNode, use, useEffect, useState } from "react";
import { shellMessages } from "./shell.messages.ts";

const pseudo = import.meta.env.DEV;

/** Compiled catalogues are separate chunks, loaded when a locale is first used (ADR-0094). */
const loadCatalogue = async (locale: string): Promise<Messages> => {
  const module = (await import(`./locales/${locale}/messages.po`)) as { messages: Messages };
  return module.messages;
};

const StudioContext = createContext<StudioI18n | null>(null);

/** The one i18n instance the app owns. */
export function useStudioI18n(): StudioI18n {
  const studio = use(StudioContext);
  if (studio === null) {
    throw new Error("useStudioI18n outside I18nProvider");
  }
  return studio;
}

/** Creates the one instance the app owns; missing translations are reported by tests, never shown. */
export function createAppI18n(): StudioI18n {
  return createStudioI18n({
    load: loadCatalogue,
    pseudo,
  });
}

/** Activates the negotiated locale, then renders the tree and keeps the document title in the catalogue. */
export function I18nProvider({ studio, children }: { studio: StudioI18n; children: ReactNode }) {
  const [ready, setReady] = useState<I18n | null>(null);
  useEffect(() => {
    const requested = typeof navigator === "undefined" ? [] : navigator.languages;
    void studio.activate(negotiateLocale({ requested, pseudo })).then((locale) => {
      document.documentElement.setAttribute("lang", locale);
      document.documentElement.setAttribute("dir", directionOf(locale));
      document.title = shellMessages(studio.i18n).appTitle;
      setReady(studio.i18n);
    });
  }, [studio]);
  return ready === null ? null : (
    <StudioContext value={studio}>
      <LinguiProvider i18n={ready}>{children}</LinguiProvider>
    </StudioContext>
  );
}
