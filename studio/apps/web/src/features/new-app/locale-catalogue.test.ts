// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { canonicalLocale, englishName, languageCodes, localeNames, searchLocales } from "./locale-catalogue.ts";

describe("[I18N-001] every language can be chosen for a new app", () => {
  test("the catalogue holds every ISO 639-1 language, Amharic included", () => {
    expect(languageCodes.length).toBeGreaterThan(180);
    expect(languageCodes).toContain("am");
  });

  test("a typed tag is made canonical, and one the documents do not take is refused", () => {
    expect(canonicalLocale("de-de")).toBe("de-DE");
    expect(canonicalLocale("am_et")).toBe("am-ET");
    expect(canonicalLocale("zh-hant-tw")).toBe("zh-Hant-TW");
    expect(canonicalLocale("not a tag")).toBeUndefined();
    expect(canonicalLocale("en-US-u-ca-buddhist")).toBeUndefined();
  });

  test("search finds by name and by code, leaves out the chosen, and offers a typed region first", () => {
    // A language is offered with its most likely regional tag, so a country's name finds it too.
    expect(searchLocales("amhar", "en", []).map((l) => l.code)).toEqual(["am", "am-ET"]);
    expect(searchLocales("Ethiopia", "en", []).map((l) => l.code)).toContain("am-ET");
    // The best match comes first: the exact code before a name that merely contains the query.
    expect(searchLocales("de", "en", [])[0]?.code).toBe("de");
    expect(searchLocales("de", "en", ["de"]).some((l) => l.code === "de")).toBe(false);
    expect(searchLocales("am-ET", "en", ["am"])[0]?.code).toBe("am-ET");
  });

  test("every language has an English name, for a browser whose ICU data has none (Chrome lacks a third)", () => {
    for (const code of languageCodes) {
      expect(englishName(code)).toBeDefined();
    }
    expect(englishName("aa")).toBe("Afar");
    expect(englishName("aa-ET")).toBe("Afar (ET)");
    expect(englishName("constructor")).toBeUndefined();
    expect(localeNames("en", "am").name).toBe("Amharic");
  });
});
