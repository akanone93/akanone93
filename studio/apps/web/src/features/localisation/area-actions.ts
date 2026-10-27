// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";
import type { Area } from "./area-scope.tsx";
import {
  addLocaleRecipe,
  type CellStatus,
  localeDocument,
  STATUSES,
  setStatesRecipe,
  writeMessagesRecipe,
} from "./grid-model.ts";
import { localePath } from "./l10n-model.ts";
import type { Entries } from "./translation-formats.ts";

/** A13 and A14 search: the plugin whose keys are shown, the locale edited, the status and the text searched. */
export const localisationSearchSchema = z.object({
  plugin: z.string().optional().catch(undefined),
  locale: z.string().optional().catch(undefined),
  status: z
    .enum(["all", ...STATUSES])
    .optional()
    .catch(undefined),
  q: z.string().optional().catch(undefined),
});
export type LocalisationSearch = z.infer<typeof localisationSearchSchema>;

const localePattern = /^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$/;

export type LocaleProblem = "invalid" | "taken";

/** Why `code` cannot be added as a locale, if it cannot. */
export function localeProblem(code: string, supported: readonly string[]): LocaleProblem | undefined {
  if (!localePattern.test(code)) {
    return "invalid";
  }
  return supported.includes(code) ? "taken" : undefined;
}

/** Writes messages with a state into the chosen locale's document, creating the document for a locale without one. */
export async function writeMessages(area: Area, texts: Entries, state: CellStatus): Promise<void> {
  const { messages, locale } = area.sessions;
  if (area.hasLocaleDoc) {
    messages.edit(localePath(locale), writeMessagesRecipe(texts, state), {
      group: `l10n-bulk:${locale}:${area.newId()}`,
    });
    return;
  }
  await messages.create(localePath(locale), localeDocument(area.schemaVersion, area.newId(), locale, texts, state));
}

/** Gives messages that exist a state. */
export function setStates(area: Area, ids: readonly string[], state: CellStatus): void {
  area.sessions.messages.edit(localePath(area.sessions.locale), setStatesRecipe(ids, state), {
    group: `l10n-state:${area.sessions.locale}:${area.newId()}`,
  });
}

/** Adds a locale to the app document and gives it an empty translations document. */
export function addLocale(area: Area, code: string): void {
  area.rememberLocale(code);
  area.sessions.keys.edit("app.json", addLocaleRecipe(code), { group: `l10n-locale:${code}` });
  area.sessions.setLocale(code);
}
