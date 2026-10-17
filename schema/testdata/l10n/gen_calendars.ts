// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Generates calendars.json, the conformance vectors of the calendar systems
// (I18N-003, ADR-0067), from the ICU that Bun bundles. Run it from this
// directory:
//
//   bun gen_calendars.ts
//
// Each case is [jdn, era, year, month, day] for the Julian day number jdn:
// the date Intl.DateTimeFormat gives in the calendar for UTC midnight of that
// day. The Go and Dart calendars (backend/internal/l10n/calendar,
// packages/plux_flutter/lib/src/l10n/calendar) must reproduce every case, and
// must convert the date back to the day number.

import { writeFileSync } from "node:fs";

const CALENDARS = ["ethiopic", "islamic-umalqura", "persian"] as const;
const UNIX_EPOCH_JDN = 2440588;
const DAY_MS = 86_400_000;

const fmt = (cal: string) =>
  new Intl.DateTimeFormat(`en-u-ca-${cal}`, {
    timeZone: "UTC",
    year: "numeric",
    month: "numeric",
    day: "numeric",
    era: "short",
  });

// Era names as ICU writes them in English, mapped to the index the engines use
// (Ethiopic 0 Amete Alem, 1 Amete Mihret; Islamic 0 AH, 1 BH; Persian 0 AP).
const ERAS: Record<string, Record<string, number>> = {
  ethiopic: { ERA0: 0, ERA1: 1, AA: 0, AM: 1 },
  "islamic-umalqura": { AH: 0, BH: 1 },
  persian: { AP: 0 },
};

function jdnToMs(jdn: number): number {
  return (jdn - UNIX_EPOCH_JDN) * DAY_MS;
}

function gregorianJdn(y: number, m: number, d: number): number {
  const dt = new Date(0);
  dt.setUTCFullYear(y, m - 1, d);
  return Math.floor(dt.getTime() / DAY_MS) + UNIX_EPOCH_JDN;
}

function convert(cal: (typeof CALENDARS)[number], f: Intl.DateTimeFormat, jdn: number): number[] {
  const parts = f.formatToParts(new Date(jdnToMs(jdn)));
  const get = (t: string) => parts.find((p) => p.type === t)!.value;
  const eraName = get("era");
  const era = ERAS[cal][eraName];
  if (era === undefined) throw new Error(`${cal}: unknown era ${eraName}`);
  const related = parts.find((p) => p.type === "relatedYear");
  if (related) throw new Error(`${cal}: relatedYear in output`);
  return [jdn, era, Number(get("year")), Number(get("month")), Number(get("day"))];
}

// Day numbers of 1900-01-01 to 2177-12-31: every 40th day, plus a band around
// the month starts of every year, the leap days, and the table edges.
const first = gregorianJdn(1900, 1, 1);
const last = gregorianJdn(2177, 12, 31);

function days(cal: (typeof CALENDARS)[number]): number[] {
  const set = new Set<number>();
  for (let j = first; j <= last; j += 40) set.add(j);
  for (let y = 1900; y <= 2177; y++) {
    for (const [m, d] of [[1, 1], [2, 28], [2, 29], [3, 1], [3, 20], [3, 21], [12, 31]] as const) {
      const j = gregorianJdn(y, m, d);
      if (j >= first && j <= last) set.add(j);
    }
  }
  const edge = (y: number, m: number, d: number, span: number) => {
    const j = gregorianJdn(y, m, d);
    for (let k = -span; k <= span; k++) set.add(j + k);
  };
  if (cal === "islamic-umalqura") {
    // 1300 AH began 1882-11-12 and 1600 AH ends 2174-11-25 (Gregorian).
    edge(1882, 11, 12, 40);
    edge(2174, 11, 25, 40);
    edge(2175, 11, 14, 5);
    edge(1900, 1, 1, 3);
  }
  if (cal === "persian") {
    // The corrected non-leap year 1502 ends 2123-03-21.
    edge(2123, 3, 21, 10);
    edge(2124, 3, 20, 10);
  }
  if (cal === "ethiopic") {
    // Amete Mihret year 1 began on JDN 1724221; the era edge, and the
    // leap-year Pagume.
    for (let k = -10; k <= 10; k++) set.add(1724221 + k);
    edge(1899, 9, 11, 10);
    edge(2007, 9, 12, 10);
    edge(2175, 9, 12, 10);
  }
  return [...set].sort((a, b) => a - b);
}

const out: Record<string, unknown> = {
  generator: "schema/testdata/l10n/gen_calendars.ts",
  icu: process.versions.icu,
  note: "Cases are [jdn, era, year, month, day]; ICU's Islamic Umm al-Qura data is the pinned table for 1300-1600 AH and tabular civil arithmetic outside it.",
};
for (const cal of CALENDARS) {
  const f = fmt(cal);
  const cases = days(cal).map((j) => convert(cal, f, j));
  out[cal] = cases;
  console.error(cal, cases.length);
}
// Ethiopic dates before the Amete Mihret era (Amete Alem), as ICU writes them.
{
  const f = fmt("ethiopic");
  const extra = new Set<number>();
  const j = 1724221;
  for (let k = -800; k <= 0; k += 17) extra.add(j + k);
  const cases = [...extra].map((x) => convert("ethiopic", f, x));
  (out["ethiopic"] as number[][]).push(...cases);
  (out["ethiopic"] as number[][]).sort((a, b) => a[0] - b[0]);
}
writeFileSync(new URL("calendars.json", import.meta.url), JSON.stringify(out) .replace(/\],\[/g, "],\n[").replace(/"(ethiopic|islamic-umalqura|persian)":\[\[/g, '\n"$1":[\n[') + "\n");
