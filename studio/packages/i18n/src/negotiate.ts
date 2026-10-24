// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { availableLocales, sourceLocale } from "./locales.ts";

/** Candidates for one tag, most specific first: `de-AT` gives `de-AT`, `de`. */
function chain(tag: string): string[] {
  const parts = tag.split("-");
  const out: string[] = [];
  for (let n = parts.length; n > 0; n--) {
    out.push(parts.slice(0, n).join("-"));
  }
  return out;
}

/** Reduces a requested tag to a served locale: `de-AT` to `de`, anything unknown to `en` (ADR-0094). */
export function resolveLocale(tag: string, pseudo = false): string {
  const served = availableLocales(pseudo).map((l) => l.tag);
  const lower = new Map(served.map((t) => [t.toLowerCase(), t]));
  for (const candidate of chain(tag.trim())) {
    const hit = lower.get(candidate.toLowerCase());
    if (hit !== undefined) {
      return hit;
    }
  }
  return sourceLocale;
}

/** Parses an `Accept-Language` header into tags ordered by quality, ties by position. */
export function parseAcceptLanguage(header: string): string[] {
  return header
    .split(",")
    .map((part, index) => {
      const [tag = "", ...params] = part.trim().split(";");
      const q = params.map((p) => p.trim()).find((p) => p.startsWith("q="));
      const quality = q === undefined ? 1 : Number(q.slice(2));
      return { tag: tag.trim(), quality: Number.isNaN(quality) ? 0 : quality, index };
    })
    .filter((e) => e.tag !== "" && e.tag !== "*" && e.quality > 0)
    .sort((a, b) => b.quality - a.quality || a.index - b.index)
    .map((e) => e.tag);
}

export interface NegotiationInput {
  /** The profile preference (STU-014), when signed in. */
  readonly profile?: string | undefined;
  /** An `Accept-Language` header value or `navigator.languages`. */
  readonly requested?: string | readonly string[] | undefined;
  readonly pseudo?: boolean | undefined;
}

/** Profile preference first, else the browser's languages in order, else `en`. */
export function negotiateLocale(input: NegotiationInput): string {
  const pseudo = input.pseudo ?? false;
  const served = new Set(availableLocales(pseudo).map((l) => l.tag.toLowerCase()));
  const matches = (tag: string): string | undefined => {
    for (const candidate of chain(tag.trim())) {
      if (served.has(candidate.toLowerCase())) {
        return resolveLocale(candidate, pseudo);
      }
    }
    return undefined;
  };
  if (input.profile !== undefined) {
    const hit = matches(input.profile);
    if (hit !== undefined) {
      return hit;
    }
  }
  const requested =
    typeof input.requested === "string" ? parseAcceptLanguage(input.requested) : [...(input.requested ?? [])];
  for (const tag of requested) {
    const hit = matches(tag);
    if (hit !== undefined) {
      return hit;
    }
  }
  return sourceLocale;
}
