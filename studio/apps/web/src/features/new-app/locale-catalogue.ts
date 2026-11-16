// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Every language an app can choose (N1): the ISO 639-1 languages, by code. A region or script is added by typing the
 * tag (`de-DE`, `am-ET`, `zh-Hant-TW`); a tag is accepted when it is a canonical BCP 47 tag of the form the documents
 * take (common.schema.json `locale`: language, optional script, optional region).
 */
export const languageCodes: readonly string[] = [
  "aa",
  "ab",
  "ae",
  "af",
  "ak",
  "am",
  "an",
  "ar",
  "as",
  "av",
  "ay",
  "az",
  "ba",
  "be",
  "bg",
  "bi",
  "bm",
  "bn",
  "bo",
  "br",
  "bs",
  "ca",
  "ce",
  "ch",
  "co",
  "cr",
  "cs",
  "cu",
  "cv",
  "cy",
  "da",
  "de",
  "dv",
  "dz",
  "ee",
  "el",
  "en",
  "eo",
  "es",
  "et",
  "eu",
  "fa",
  "ff",
  "fi",
  "fj",
  "fo",
  "fr",
  "fy",
  "ga",
  "gd",
  "gl",
  "gn",
  "gu",
  "gv",
  "ha",
  "he",
  "hi",
  "ho",
  "hr",
  "ht",
  "hu",
  "hy",
  "hz",
  "ia",
  "id",
  "ie",
  "ig",
  "ii",
  "ik",
  "io",
  "is",
  "it",
  "iu",
  "ja",
  "jv",
  "ka",
  "kg",
  "ki",
  "kj",
  "kk",
  "kl",
  "km",
  "kn",
  "ko",
  "kr",
  "ks",
  "ku",
  "kv",
  "kw",
  "ky",
  "la",
  "lb",
  "lg",
  "li",
  "ln",
  "lo",
  "lt",
  "lu",
  "lv",
  "mg",
  "mh",
  "mi",
  "mk",
  "ml",
  "mn",
  "mr",
  "ms",
  "mt",
  "my",
  "na",
  "nb",
  "nd",
  "ne",
  "ng",
  "nl",
  "nn",
  "no",
  "nr",
  "nv",
  "ny",
  "oc",
  "oj",
  "om",
  "or",
  "os",
  "pa",
  "pi",
  "pl",
  "ps",
  "pt",
  "qu",
  "rm",
  "rn",
  "ro",
  "ru",
  "rw",
  "sa",
  "sc",
  "sd",
  "se",
  "sg",
  "si",
  "sk",
  "sl",
  "sm",
  "sn",
  "so",
  "sq",
  "sr",
  "ss",
  "st",
  "su",
  "sv",
  "sw",
  "ta",
  "te",
  "tg",
  "th",
  "ti",
  "tk",
  "tl",
  "tn",
  "to",
  "tr",
  "ts",
  "tt",
  "tw",
  "ty",
  "ug",
  "uk",
  "ur",
  "uz",
  "ve",
  "vi",
  "vo",
  "wa",
  "wo",
  "xh",
  "yi",
  "yo",
  "za",
  "zh",
  "zu",
];

/** The documents' locale form (common.schema.json `locale`). */
const LOCALE = /^[a-z]{2,3}(-[A-Z][a-z]{3})?(-([A-Z]{2}|[0-9]{3}))?$/;

/** The canonical form of a typed tag (`de-de` → `de-DE`), or undefined when it is not one the documents take. */
export function canonicalLocale(typed: string): string | undefined {
  const t = typed.trim().replaceAll("_", "-");
  if (t === "") {
    return undefined;
  }
  try {
    const [canonical] = Intl.getCanonicalLocales(t);
    return canonical !== undefined && LOCALE.test(canonical) && canonical.length <= 16 ? canonical : undefined;
  } catch {
    return undefined;
  }
}

/** A language's name in `uiLocale`, and in the language itself; the code when the browser has no name for it. */
export function localeNames(uiLocale: string, code: string): { readonly name: string; readonly native: string } {
  const of = (locale: string) => {
    try {
      return new Intl.DisplayNames([locale], { type: "language" }).of(code) ?? code;
    } catch {
      return code;
    }
  };
  return { name: of(uiLocale), native: of(code) };
}

/**
 * The languages that match `query` by code or by either name, the chosen ones left out, sorted by name in the UI
 * language; a typed tag that is valid and not listed (a region such as `am-ET`) comes first.
 */
export function searchLocales(
  query: string,
  uiLocale: string,
  chosen: readonly string[],
): readonly { readonly code: string; readonly name: string; readonly native: string }[] {
  const q = query.trim().toLowerCase();
  const all = languageCodes
    .filter((code) => !chosen.includes(code))
    .map((code) => ({ code, ...localeNames(uiLocale, code) }))
    .filter(
      (l) =>
        q === "" ||
        l.code.toLowerCase().startsWith(q) ||
        l.name.toLowerCase().includes(q) ||
        l.native.toLowerCase().includes(q),
    )
    .sort((a, b) => a.name.localeCompare(b.name, uiLocale));
  const typed = canonicalLocale(query);
  if (typed !== undefined && !chosen.includes(typed) && !all.some((l) => l.code === typed)) {
    return [{ code: typed, ...localeNames(uiLocale, typed) }, ...all];
  }
  return all;
}
