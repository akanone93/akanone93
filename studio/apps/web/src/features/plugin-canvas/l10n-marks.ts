// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Localisation on the canvas (PC-32, I18N-001, I18N-006, I18N-007, I18N-012): what the Localisation view marks on the
 * screens — a text whose key has no message in the previewed locale (it falls back to the default locale's), a text
 * written in the page rather than as a key (it is not translated), and a text that overflows its box in this locale —
 * and the per-locale overrides of a widget's props, applied to the drawn copy in the previewed locale.
 */

import { readKeys, readLocales, readMessages } from "../localisation/l10n-model.ts";
import { type DocLike, jsonOf, localeChain, TEXT_PROPS } from "./look.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** What the Localisation view says of a text. */
export type L10nMark = "missing" | "literal" | "overflow";

/** The pseudo-locales (I18N-010): they show the default locale's messages transformed, so nothing is missing there. */
const PSEUDO: ReadonlySet<string> = new Set(["en-XA", "ar-XB"]);

/**
 * The keys with no message of their own in `locale` (I18N-006): the device shows the next locale's along the chain
 * (`de-AT`, `de`, the default), so a key is missing in `de-AT` when neither `de-AT` nor `de` has it, and missing in
 * the default locale when it has no message at all.
 */
export function missingKeys(documents: readonly DocLike[], locale: string): Set<string> {
  const app = jsonOf(documents, "app")[0];
  const fallback = readLocales(app)?.defaultLocale ?? "en";
  const shown = PSEUDO.has(locale) ? fallback : locale;
  const byLocale = new Map(jsonOf(documents, "translations").map((d) => [String(d["locale"]), readMessages(d)]));
  const all = new Set([
    ...jsonOf(documents, "translationKeys").flatMap((d) => readKeys(d).map((k) => k.id)),
    ...Object.keys(byLocale.get(fallback) ?? {}),
  ]);
  const own = localeChain(shown, shown).map((l) => byLocale.get(l) ?? {});
  return new Set([...all].filter((k) => own.every((m) => m[k] === undefined || m[k] === "")));
}

/** Whether an app has more than one locale, so a literal text is left untranslated somewhere. */
export function multilingual(documents: readonly DocLike[]): boolean {
  const app = jsonOf(documents, "app")[0];
  return (readLocales(app)?.supported.length ?? 1) > 1;
}

const kidsOf = (n: Obj): unknown[] => [
  ...(isObj(n["slots"]) ? Object.values(n["slots"]).flatMap((v) => (Array.isArray(v) ? v : [v])) : []),
  ...(Array.isArray(n["children"]) ? n["children"] : []),
];

/** What the view says of node `n`'s texts: a key missing in the locale wins over a literal. */
function markOf(n: Obj, missing: ReadonlySet<string>, literals: boolean): L10nMark | undefined {
  const props = isObj(n["props"]) ? n["props"] : {};
  let mark: L10nMark | undefined;
  for (const [k, v] of Object.entries(props)) {
    if (!TEXT_PROPS.has(k)) {
      continue;
    }
    if (isObj(v) && typeof v["$t"] === "string" && missing.has(v["$t"])) {
      return "missing";
    }
    if (literals && typeof v === "string" && v.trim() !== "") {
      mark = "literal";
    }
  }
  return mark;
}

/** The marks of the texts on `roots`; the laid-out widgets whose text overflows in this locale win over the others. */
export function l10nMarks(
  roots: readonly unknown[],
  missing: ReadonlySet<string>,
  literals: boolean,
  overflowing: Iterable<string>,
): Map<string, L10nMark> {
  const out = new Map<string, L10nMark>();
  const visit = (n: unknown) => {
    if (!isObj(n)) {
      return;
    }
    const mark = typeof n["id"] === "string" ? markOf(n, missing, literals) : undefined;
    if (mark !== undefined) {
      out.set(n["id"] as string, mark);
    }
    kidsOf(n).forEach(visit);
  };
  roots.forEach(visit);
  for (const id of overflowing) {
    out.set(id, "overflow");
  }
  return out;
}

/** The Localisation view's marks with their words, as the overlay draws them. */
export interface L10nOverlay {
  readonly marks: ReadonlyMap<string, L10nMark>;
  readonly text: (mark: L10nMark) => string;
}

/** The widgets the layout found overflowing, as one sorted key: an unchanged layout keeps the same key. */
export function overflowKeyOf(laidOut: readonly { readonly overflowing?: readonly string[] }[]): string {
  return [...new Set(laidOut.flatMap((h) => h.overflowing ?? []))].sort().join("\n");
}
