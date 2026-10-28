// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  calendarSystem,
  calendarSystemOf,
  calendarWeekday,
  ethiopicId,
  islamicUmalquraId,
  jdnOfIsoDate,
  persianId,
  secondaryTitle,
} from "./calendar.ts";
import {
  civilEpochJdn,
  persianEpochJdn,
  persianNonLeapYears,
  umalquraEpochDay,
  umalquraMonthMasks,
  umalquraYearEnd,
  umalquraYearStart,
} from "./calendar-tables.gen.ts";

const schema = join(import.meta.dir, "../../../../../schema/l10n/calendars");
const json = (file: string): Record<string, unknown> => JSON.parse(readFileSync(join(schema, file), "utf8"));

describe("calendar systems of DualCalendarDatePicker [I18N-003] [STU-005]", () => {
  test("the tables equal the JSON the runtime's tables are generated from", () => {
    const umalqura = json("umalqura.json");
    expect(umalquraYearStart).toBe(umalqura["yearStart"] as number);
    expect(umalquraYearEnd).toBe(umalqura["yearEnd"] as number);
    expect(civilEpochJdn).toBe(umalqura["civilEpochJdn"] as number);
    expect(umalquraEpochDay).toBe(umalqura["epochDay1300"] as number);
    expect([...umalquraMonthMasks]).toEqual(umalqura["monthMasks"] as number[]);
    const persian = json("persian.json");
    expect(persianEpochJdn).toBe(persian["epochJdn"] as number);
    expect([...persianNonLeapYears]).toEqual(persian["nonLeapYears"] as number[]);
  });

  test("converts a day between the four systems", () => {
    const jdn = jdnOfIsoDate("2024-03-15") ?? 0;
    expect(calendarSystem(islamicUmalquraId)?.fromJdn(jdn)).toEqual({ year: 1445, month: 9, day: 5 });
    expect(calendarSystem(persianId)?.fromJdn(jdn)).toEqual({ year: 1402, month: 12, day: 25 });
    expect(calendarSystem(ethiopicId)?.fromJdn(jdn)).toEqual({ year: 2016, month: 7, day: 6 });
    expect(calendarWeekday(jdn)).toBe(5);
    for (const id of [ethiopicId, islamicUmalquraId, persianId, "gregorian"]) {
      const system = calendarSystem(id);
      expect(system?.toJdn(system.fromJdn(jdn))).toBe(jdn);
    }
  });

  test("names a system by the enum member and reads document dates", () => {
    expect(calendarSystemOf("islamicUmalqura")?.id).toBe(islamicUmalquraId);
    expect(calendarSystemOf("nope")).toBeUndefined();
    expect(jdnOfIsoDate("2024-3-15")).toBeUndefined();
  });

  test("titles a month by the secondary calendar's span", () => {
    const umalqura = calendarSystem(islamicUmalquraId);
    const start = jdnOfIsoDate("2024-03-01") ?? 0;
    expect(umalqura && secondaryTitle(umalqura, start, start + 30)).toBe("Shaʻban – Ramadan 1445 AH");
  });
});
