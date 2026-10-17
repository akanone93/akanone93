// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Generates the conformance vectors of the localisation engines
// (plurals.json, numbers.json, money.json, dates.json) from the ICU that Bun
// bundles (Intl.PluralRules, Intl.NumberFormat, Intl.DateTimeFormat). Run it
// from this directory:
//
//   bun gen.ts
//
// The Go engine (backend/internal/l10n) and the Dart engine
// (packages/plux_flutter/lib/src/l10n) run every case (QA-003, I18N-001,
// I18N-011). The expected strings are ICU's; the engines read CLDR 48.2.3, so
// a case where ICU's CLDR differs carries a "note" and the engines' value in
// "ours" (the engines then expect "ours"). Rounding is half-even everywhere
// (ADR-0066), which Intl spells roundingMode "halfEven".

import { writeFileSync } from "node:fs";

const LOCALES = [
  "en", "de-DE", "ar", "am-ET",
  "fr", "es", "hi", "ja", "zh", "ru", "tr", "fa", "he", "th", "sw",
  "pt", "it", "ko", "id", "bn", "ur", "pl", "nl", "uk",
];

// Locales whose own numbering system is not Latin, to exercise native digits.
const NATIVE = ["fa", "bn", "my", "mr", "ne"].filter((l) => LOCALES.includes(l) || l === "mr" || l === "ne" || l === "my");

type Json = Record<string, unknown>;

function dump(file: string, kind: string, cases: Json[]): void {
  const lines = cases.map((c) => "    " + JSON.stringify(c));
  const text =
    `{\n  "kind": ${JSON.stringify(kind)},${kind === "dates" ? '\n  "fields": "date is [era, year, month, day, weekday, hour, minute, second]; calendar defaults to gregorian",\n  "normalize": "JavaScriptCore writes U+0020 where CLDR has U+202F; engines are compared after replacing U+202F by U+0020",' : ""}\n  "icu": ${JSON.stringify(process.versions.icu)},\n  "cldr": "48.2.3",\n  "cases": [\n` +
    lines.join(",\n") +
    "\n  ]\n}\n";
  // Escape invisible and space-like characters so they stay reviewable.
  const escaped = text.replace(/[\p{Z}\p{Cc}\p{Cf}]/gu, (ch) =>
    ch === " " || ch === "\n" ? ch : "\\u" + ch.charCodeAt(0).toString(16).padStart(4, "0"),
  );
  writeFileSync(new URL(file, import.meta.url), escaped);
  console.log(file, cases.length, "cases");
}

// ---- plurals ----

function fractionDigits(s: string): number {
  const i = s.indexOf(".");
  return i < 0 ? 0 : s.length - i - 1;
}

function plurals(): Json[] {
  const cardinal = [
    "0", "1", "2", "3", "4", "5", "6", "7", "10", "11", "12", "13", "14", "20", "21", "22", "25", "99",
    "100", "101", "102", "111", "200", "1000", "1000000", "0.0", "0.5", "1.0", "1.5", "2.0", "2.5", "1.00", "10.0", "12.34",
  ];
  const ordinal = ["1", "2", "3", "4", "5", "8", "11", "12", "13", "21", "22", "23", "24", "81", "101"];
  const out: Json[] = [];
  for (const locale of LOCALES) {
    for (const n of cardinal) {
      const d = fractionDigits(n);
      const pr = new Intl.PluralRules(locale, { type: "cardinal", minimumFractionDigits: d, maximumFractionDigits: d });
      out.push({ locale, type: "cardinal", n, expect: pr.select(Number(n)) });
    }
    for (const n of ordinal) {
      const pr = new Intl.PluralRules(locale, { type: "ordinal" });
      out.push({ locale, type: "ordinal", n, expect: pr.select(Number(n)) });
    }
  }
  return out;
}

// ---- numbers ----

function numbers(): Json[] {
  const out: Json[] = [];
  const decimal = ["0", "7", "1234", "12345", "1234.5", "1234567.891", "-1234567.891", "0.5", "-0.25", "123456789012.345", "2.5", "3.5", "0.125"];
  const percent = ["0", "0.256", "1.5", "-0.05", "12.345"];
  const compact = ["0", "7", "999", "1000", "1234", "1500", "9999", "12345", "99999", "123456", "999999", "1234567", "15000000", "1500000000", "1234567890123", "-4500", "0.0123", "1.5", "12.5"];
  const locales = [...LOCALES, ...["mr", "ne", "my"]];
  for (const locale of locales) {
    const native = locale === "mr" || locale === "ne" || locale === "my" || NATIVE.includes(locale);
    for (const value of decimal) {
      const nf = new Intl.NumberFormat(locale, { numberingSystem: "latn", roundingMode: "halfEven" } as Intl.NumberFormatOptions);
      out.push({ locale, op: "decimal", value, expect: nf.format(Number(value)) });
    }
    for (const [min, max] of [[2, 2], [0, 1], [1, 3]]) {
      const nf = new Intl.NumberFormat(locale, { numberingSystem: "latn", minimumFractionDigits: min, maximumFractionDigits: max, roundingMode: "halfEven" } as Intl.NumberFormatOptions);
      for (const value of ["1234.5", "0.005", "2.675", "1234567.891"]) {
        out.push({ locale, op: "decimal", value, options: { minFraction: min, maxFraction: max }, expect: nf.format(Number(value)) });
      }
    }
    for (const value of percent) {
      const nf = new Intl.NumberFormat(locale, { style: "percent", numberingSystem: "latn", roundingMode: "halfEven" } as Intl.NumberFormatOptions);
      out.push({ locale, op: "percent", value, expect: nf.format(Number(value)) });
    }
    for (const value of compact) {
      const nf = new Intl.NumberFormat(locale, { notation: "compact", compactDisplay: "short", numberingSystem: "latn", roundingMode: "halfEven" } as Intl.NumberFormatOptions);
      out.push({ locale, op: "compact", value, expect: nf.format(Number(value)) });
    }
    if (native) {
      for (const value of ["1234567.891", "-0.5"]) {
        const nf = new Intl.NumberFormat(locale, { roundingMode: "halfEven" } as Intl.NumberFormatOptions);
        out.push({ locale, op: "decimal", value, options: { numbering: "default" }, expect: nf.format(Number(value)) });
        const pf = new Intl.NumberFormat(locale, { style: "percent", roundingMode: "halfEven" } as Intl.NumberFormatOptions);
        out.push({ locale, op: "percent", value: "0.256", options: { numbering: "default" }, expect: pf.format(0.256) });
      }
    }
  }
  return out;
}

// ---- money ----

function money(): Json[] {
  const out: Json[] = [];
  const amounts: Array<[string, string]> = [
    ["EUR", "1234.56"], ["USD", "1234.56"], ["JPY", "1234.5"], ["GBP", "0.5"], ["INR", "1234567.891"],
    ["CHF", "-1234.5"], ["KWD", "1.2345"], ["BRL", "99.995"], ["CNY", "1234.565"], ["USD", "0"], ["EUR", "-0.001"],
  ];
  for (const locale of LOCALES) {
    for (const [currency, amount] of amounts) {
      for (const display of ["symbol", "narrowSymbol", "code"]) {
        const nf = new Intl.NumberFormat(locale, { style: "currency", currency, currencyDisplay: display, numberingSystem: "latn", roundingMode: "halfEven" } as Intl.NumberFormatOptions);
        out.push({ locale, currency, amount, display: display === "narrowSymbol" ? "narrow" : display, expect: nf.format(Number(amount)) });
      }
    }
    for (const [currency, amount] of [["USD", "-1234.56"], ["EUR", "-5"], ["USD", "1234.56"]] as Array<[string, string]>) {
      const nf = new Intl.NumberFormat(locale, { style: "currency", currency, currencySign: "accounting", numberingSystem: "latn", roundingMode: "halfEven" } as Intl.NumberFormatOptions);
      out.push({ locale, currency, amount, display: "symbol", accounting: true, expect: nf.format(Number(amount)) });
    }
    const nf = new Intl.NumberFormat(locale, { style: "currency", currency: "USD", minimumFractionDigits: 0, maximumFractionDigits: 0, numberingSystem: "latn", roundingMode: "halfEven" } as Intl.NumberFormatOptions);
    out.push({ locale, currency: "USD", amount: "1234.5", display: "symbol", options: { minFraction: 0, maxFraction: 0 }, expect: nf.format(1234.5) });
  }
  for (const locale of ["fa", "bn", "mr"]) {
    const nf = new Intl.NumberFormat(locale, { style: "currency", currency: "USD", roundingMode: "halfEven" } as Intl.NumberFormatOptions);
    out.push({ locale, currency: "USD", amount: "1234.56", display: "symbol", options: { numbering: "default" }, expect: nf.format(1234.56) });
  }
  return out;
}

// ---- dates ----

const DATES: Array<[number, number, number, number, number, number]> = [
  [2025, 10, 16, 15, 4, 5],
  [2024, 2, 29, 0, 30, 0],
  [2025, 7, 4, 12, 5, 9],
  [2025, 12, 31, 23, 59, 59],
  [2026, 1, 1, 9, 8, 7],
];

// Skeletons and the Intl options that ICU turns into them. Minutes are mm
// (minute: "2-digit"). A numeric 24-hour hour in a locale whose own cycle is
// 24-hour is written with one digit whatever the pattern says, a quirk of the
// JavaScript engine rather than of CLDR, so the vectors ask for two-digit
// hours with h23.
const SKELETONS: Record<string, Intl.DateTimeFormatOptions> = {
  yMMMMd: { year: "numeric", month: "long", day: "numeric" },
  yMMMd: { year: "numeric", month: "short", day: "numeric" },
  yMd: { year: "numeric", month: "numeric", day: "numeric" },
  yMMMEd: { year: "numeric", month: "short", day: "numeric", weekday: "short" },
  yMMMMEEEEd: { year: "numeric", month: "long", day: "numeric", weekday: "long" },
  yMMMM: { year: "numeric", month: "long" },
  yM: { year: "numeric", month: "numeric" },
  MMMd: { month: "short", day: "numeric" },
  MMMMd: { month: "long", day: "numeric" },
  Md: { month: "numeric", day: "numeric" },
  y: { year: "numeric" },
  MMMM: { month: "long" },
  EEEE: { weekday: "long" },
  d: { day: "numeric" },
  HHmm: { hour: "2-digit", minute: "2-digit", hourCycle: "h23" },
  HHmmss: { hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23" },
  hmm: { hour: "numeric", minute: "2-digit", hourCycle: "h12" },
  hhmm: { hour: "2-digit", minute: "2-digit", hourCycle: "h12" },
  hmmss: { hour: "numeric", minute: "2-digit", second: "2-digit", hourCycle: "h12" },
  yMMMMdHHmm: { year: "numeric", month: "long", day: "numeric", hour: "2-digit", minute: "2-digit", hourCycle: "h23" },
  yMdhmm: { year: "numeric", month: "numeric", day: "numeric", hour: "numeric", minute: "2-digit", hourCycle: "h12" },
  yMMMEdHHmm: { year: "numeric", month: "short", day: "numeric", weekday: "short", hour: "2-digit", minute: "2-digit", hourCycle: "h23" },
};

type Fields = number[];

function utc(d: [number, number, number, number, number, number]): Date {
  return new Date(Date.UTC(d[0], d[1] - 1, d[2], d[3], d[4], d[5]));
}

// Calendar fields of an instant in a calendar, read from Intl itself.
function fieldsOf(date: Date, calendar: string): Fields {
  const f = new Intl.DateTimeFormat(`en-u-ca-${calendar === "gregorian" ? "gregory" : calendar}-nu-latn`, {
    timeZone: "UTC", era: "short", year: "numeric", month: "numeric", day: "numeric",
  });
  const p: Record<string, string> = {};
  for (const part of f.formatToParts(date)) p[part.type] = part.value;
  let era = 0;
  if (calendar === "gregorian") era = p.era === "AD" ? 1 : 0;
  else if (calendar === "ethiopic") era = p.era === "ERA0" || p.era === "AA" ? 0 : 1;
  const year = Number(p.relatedYear ?? p.year);
  return [era, year, Number(p.month), Number(p.day), date.getUTCDay(), date.getUTCHours(), date.getUTCMinutes(), date.getUTCSeconds()];
}

function dates(): Json[] {
  const out: Json[] = [];
  const styles = ["full", "long", "medium", "short"] as const;
  for (const locale of LOCALES) {
    for (const d of DATES.slice(0, 3)) {
      const date = utc(d);
      const fields = fieldsOf(date, "gregorian");
      const intl = (o: Intl.DateTimeFormatOptions) =>
        new Intl.DateTimeFormat(`${locale}-u-ca-gregory-nu-latn`, { timeZone: "UTC", ...o }).format(date);
      for (const s of styles) {
        out.push({ locale, date: fields, dateStyle: s, expect: intl({ dateStyle: s }) });
      }
      for (const s of ["short", "medium"] as const) {
        out.push({ locale, date: fields, timeStyle: s, expect: intl({ timeStyle: s }) });
      }
      for (const [ds, ts] of [["long", "short"], ["medium", "medium"], ["short", "short"], ["full", "medium"]] as const) {
        out.push({ locale, date: fields, dateStyle: ds, timeStyle: ts, expect: intl({ dateStyle: ds, timeStyle: ts }) });
      }
      for (const [skeleton, opts] of Object.entries(SKELETONS)) {
        out.push({ locale, date: fields, skeleton, expect: intl(opts) });
      }
    }
  }
  // Other calendars: the Gregorian instant converted by Intl, formatted by Intl.
  const calendars = ["ethiopic", "islamic-umalqura", "persian"];
  const calLocales = ["en", "de-DE", "ar", "am-ET", "fa", "he", "hi", "ja", "zh", "ru"];
  for (const calendar of calendars) {
    for (const locale of calLocales) {
      for (const d of DATES.slice(0, 2)) {
        const date = utc(d);
        const fields = fieldsOf(date, calendar);
        const intl = (o: Intl.DateTimeFormatOptions) =>
          new Intl.DateTimeFormat(`${locale}-u-ca-${calendar === "gregorian" ? "gregory" : calendar}-nu-latn`, { timeZone: "UTC", ...o }).format(date);
        for (const s of styles) {
          out.push({ locale, calendar, date: fields, dateStyle: s, expect: intl({ dateStyle: s }) });
        }
        for (const skeleton of ["yMMMMd", "yMMMd", "yMd", "MMMd", "yMMMEd"]) {
          const c: Json = { locale, calendar, date: fields, skeleton, expect: intl(SKELETONS[skeleton]) };
          if (locale === "fa" && calendar === "persian" && skeleton === "yMMMEd") {
            // ICU takes this skeleton's pattern (y MMM d, E) from the root
            // locale; the cldr-json data of fa defines only yyyyMMMEd (E d MMM y).
            const parts = new Intl.DateTimeFormat(`fa-u-ca-persian-nu-latn`, { timeZone: "UTC", ...SKELETONS[skeleton] }).formatToParts(date);
            const v = (t: string) => parts.find((p) => p.type === t)!.value;
            c.note = "ICU inherits the yMMMEd pattern from root; the CLDR data of fa defines yyyyMMMEd only";
            c.ours = [v("weekday"), v("day"), v("month"), v("year")].join(" ");
          }
          out.push(c);
        }
      }
    }
  }
  // Native digits in dates.
  for (const locale of ["fa", "bn", "mr"]) {
    for (const d of DATES.slice(0, 2)) {
      const date = utc(d);
      const fields = fieldsOf(date, "gregorian");
      const f = (o: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat(`${locale}-u-ca-gregory`, { timeZone: "UTC", ...o }).format(date);
      out.push({ locale, date: fields, numbering: "default", dateStyle: "long", expect: f({ dateStyle: "long" }) });
      out.push({ locale, date: fields, numbering: "default", skeleton: "yMd", expect: f(SKELETONS.yMd) });
    }
  }
  return out;
}

dump("plurals.json", "plurals", plurals());
dump("numbers.json", "numbers", numbers());
dump("money.json", "money", money());
dump("dates.json", "dates", dates());
