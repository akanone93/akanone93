// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { canonicalLocale, languageCodes, searchLocales } from "./locale-catalogue.ts";

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
    expect(searchLocales("amhar", "en", []).map((l) => l.code)).toEqual(["am"]);
    expect(searchLocales("de", "en", ["de"]).some((l) => l.code === "de")).toBe(false);
    expect(searchLocales("am-ET", "en", ["am"])[0]?.code).toBe("am-ET");
  });
});
