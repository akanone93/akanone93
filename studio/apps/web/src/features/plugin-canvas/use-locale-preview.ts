// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The previewed locale on the canvas (PC-32): the plural samples picked in the inspector, what the Localisation tab
 * knows of the locale's messages, and the Localisation view's marks — missing and untranslated texts, and texts the
 * layout found overflowing their box in this locale.
 */

import { useCallback, useMemo, useState } from "react";
import { readLocales } from "../localisation/l10n-model.ts";
import type { LocalePreview } from "../panels/locale-tools.tsx";
import type { PreviewRender } from "../preview/switches.ts";
import { format } from "./icu.ts";
import { type L10nMark, type L10nOverlay, l10nMarks, missingKeys, multilingual } from "./l10n-marks.ts";
import { type DocLike, jsonOf, messagesFor } from "./look.ts";

/** The count each plural text is drawn with, by node, and its setter; undefined takes the count from the data. */
export function usePluralSamples(): {
  readonly samples: ReadonlyMap<string, number>;
  readonly setSample: (node: string, n: number | undefined) => void;
} {
  const [samples, setSamples] = useState<ReadonlyMap<string, number>>(new Map());
  const setSample = useCallback((node: string, n: number | undefined) => {
    setSamples((prev) => {
      const next = new Map(prev);
      if (n === undefined) {
        next.delete(node);
      } else {
        next.set(node, n);
      }
      return next;
    });
  }, []);
  return { samples, setSample };
}

/** The words of the view's marks. */
export interface L10nMarkText {
  readonly missing: (locale: string) => string;
  readonly literal: string;
  readonly overflow: string;
}

export function useLocalePreview(input: {
  readonly documents: readonly DocLike[];
  readonly look: PreviewRender;
  readonly roots: readonly unknown[];
  readonly view: boolean;
  /** The overflowing widgets' key from `overflowKeyOf`. */
  readonly overflowKey: string;
  readonly samples: ReturnType<typeof usePluralSamples>;
  readonly text: L10nMarkText;
}): { readonly preview: LocalePreview; readonly marks: L10nOverlay | undefined } {
  const { documents, look, roots, view, overflowKey, samples, text } = input;
  const missing = useMemo(() => missingKeys(documents, look.locale), [documents, look.locale]);
  const messages = useMemo(() => messagesFor(documents, look), [documents, look]);
  const defaultLocale = useMemo(() => readLocales(jsonOf(documents, "app")[0])?.defaultLocale ?? "en", [documents]);
  const preview = useMemo<LocalePreview>(
    () => ({
      locale: look.locale,
      defaultLocale,
      messageOf: (key) => messages[key],
      textOf: (key) => {
        const message = messages[key];
        return message === undefined ? undefined : format(message, {}, look.locale);
      },
      missing: (key) => missing.has(key),
      sampleOf: (node) => samples.samples.get(node),
      setSample: samples.setSample,
    }),
    [look.locale, defaultLocale, messages, missing, samples],
  );
  const marks = useMemo<L10nOverlay | undefined>(() => {
    if (!view) {
      return undefined;
    }
    const overflowing = overflowKey === "" ? [] : overflowKey.split("\n");
    const words = (mark: L10nMark) =>
      mark === "missing" ? text.missing(look.locale) : mark === "literal" ? text.literal : text.overflow;
    return { marks: l10nMarks(roots, missing, multilingual(documents), overflowing), text: words };
  }, [view, overflowKey, roots, missing, documents, text, look.locale]);
  return { preview, marks };
}
