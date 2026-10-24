// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  availableLocales,
  createFormatters,
  createStudioI18n,
  directionOf,
  negotiateLocale,
  parseAcceptLanguage,
  pseudoAccent,
  pseudoMirror,
  resolveLocale,
} from "./index.ts";

const catalogues: Record<string, Record<string, string>> = {
  en: { "shell.app.title": "Plux Studio", "shell.app.tagline": "Design apps" },
  de: { "shell.app.title": "Plux Studio" },
  ar: { "shell.app.title": "استوديو Plux", "shell.app.tagline": "صمم" },
};

function make(missing: string[] = []) {
  const root = document.createElement("html");
  const studio = createStudioI18n({
    load: async (locale) => catalogues[locale] ?? {},
    pseudo: true,
    root,
    onMissing: (locale, id) => missing.push(`${locale}:${id}`),
  });
  return { studio, root };
}

describe("locale negotiation and fallback [STU-012]", () => {
  test("de-AT falls back to de, then en", () => {
    expect(resolveLocale("de-AT")).toBe("de");
    expect(resolveLocale("fr-CA")).toBe("en");
    expect(resolveLocale("AR")).toBe("ar");
  });

  test("profile preference wins over Accept-Language", () => {
    expect(negotiateLocale({ profile: "ar", requested: "de" })).toBe("ar");
    expect(negotiateLocale({ profile: "xx", requested: "de-AT,en;q=0.5" })).toBe("de");
  });

  test("Accept-Language is ordered by quality and navigator.languages by position", () => {
    expect(parseAcceptLanguage("en;q=0.4, de;q=0.9, *;q=0.1")).toEqual(["de", "en"]);
    expect(negotiateLocale({ requested: ["fr", "ar-EG", "de"] })).toBe("ar");
    expect(negotiateLocale({})).toBe("en");
  });

  test("pseudo-locales are offered only on request", () => {
    expect(availableLocales(false).map((l) => l.tag)).toEqual(["en", "de", "ar"]);
    expect(resolveLocale("en-XA")).toBe("en");
    expect(resolveLocale("en-XA", true)).toBe("en-XA");
  });
});

describe("direction [STU-012]", () => {
  test("ar and ar-XB are right-to-left, the rest left-to-right", () => {
    expect(directionOf("ar")).toBe("rtl");
    expect(directionOf("ar-XB")).toBe("rtl");
    expect(directionOf("ar-EG")).toBe("rtl");
    expect(directionOf("en")).toBe("ltr");
    expect(directionOf("de-AT")).toBe("ltr");
    expect(directionOf("en-XA")).toBe("ltr");
  });
});

describe("pseudo-locales [I18N-010]", () => {
  test("en-XA accents, expands by 40 percent and keeps placeholders", () => {
    const out = pseudoAccent("Hello {name}");
    expect(out).toContain("{name}");
    expect(out).not.toContain("Hello");
    expect(out.length).toBeGreaterThanOrEqual("Hello {name}".length * 1.3);
  });

  test("ar-XB wraps text in right-to-left marks and keeps placeholders", () => {
    const out = pseudoMirror("Save {count}");
    expect(out.startsWith("‫")).toBe(true);
    expect(out.endsWith("‬")).toBe(true);
    expect(out).toContain("{count}");
    expect(out).toContain("‏");
  });
});

describe("switching locale [STU-012]", () => {
  test("updates lang and dir without a reload and translates", async () => {
    const { studio, root } = make();
    await studio.activate("en");
    expect(root.lang).toBe("en");
    expect(root.dir).toBe("ltr");
    await studio.activate("ar");
    expect(root.lang).toBe("ar");
    expect(root.dir).toBe("rtl");
    expect(studio.i18n._("shell.app.title")).toBe("استوديو Plux");
    await studio.activate("ar-XB");
    expect(root.dir).toBe("rtl");
    expect(studio.i18n._("shell.app.title").startsWith("‫")).toBe(true);
    await studio.activate("en-XA");
    expect(root.dir).toBe("ltr");
    expect(studio.i18n._("shell.app.title")).not.toBe("Plux Studio");
  });

  test("a missing translation is reported and never shown as a key", async () => {
    const missing: string[] = [];
    const { studio } = make(missing);
    await studio.activate("de");
    expect(missing).toContain("de:shell.app.tagline");
    expect(studio.i18n._("shell.app.tagline")).toBe("Design apps");
    expect(studio.i18n._("shell.app.unknown")).toBe("");
  });
});

describe("Intl helpers per locale [STU-012]", () => {
  test("numbers, dates, relative times and lists follow the locale", () => {
    expect(createFormatters("de").number(1234.5)).toBe("1.234,5");
    expect(createFormatters("en").number(1234.5)).toBe("1,234.5");
    expect(createFormatters("ar").list(["a", "b"])).toBe("a وb");
    expect(createFormatters("en").date(Date.UTC(2026, 0, 2), { timeZone: "UTC", dateStyle: "short" })).toBe("1/2/26");
    expect(createFormatters("de").relativeTime(-1, "day", { numeric: "auto" })).toBe("gestern");
    expect(createFormatters("en").list(["a", "b", "c"])).toBe("a, b, and c");
    expect(createFormatters("de").list(["a", "b", "c"])).toBe("a, b und c");
    expect(createFormatters("en-XA").number(1234.5)).toBe("1,234.5");
  });
});
