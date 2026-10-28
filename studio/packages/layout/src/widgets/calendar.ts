// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The calendar systems of `DualCalendarDatePicker` (I18N-003, ADR-0067), ported from the runtime's
 * `packages/plux_flutter/lib/src/l10n/calendar/calendar.dart` so Studio lays out the same month grid and the same
 * secondary-calendar title. Dates are `{ year, month, day }` and days are Julian day numbers.
 */

import {
  civilEpochJdn,
  persianEpochJdn,
  persianNonLeapYears,
  umalquraEpochDay,
  umalquraMonthMasks,
  umalquraYearEnd,
  umalquraYearStart,
} from "./calendar-tables.gen.ts";

/** A date in some calendar. */
export interface CalendarYmd {
  readonly year: number;
  readonly month: number;
  readonly day: number;
}

/** A calendar system, as the runtime's `CalendarSystem`. */
export interface CalendarSystem {
  readonly id: string;
  toJdn(d: CalendarYmd): number;
  fromJdn(jdn: number): CalendarYmd;
  daysInMonth(year: number, month: number): number;
  /** The era (an index into the system's era names) and the year in it. */
  eraOf(year: number): { era: number; year: number };
}

/** The permanent ids of the systems. */
export const gregorianId = "gregorian";
export const ethiopicId = "ethiopic";
export const islamicUmalquraId = "islamic-umalqura";
export const persianId = "persian";

/** Floor division, as the runtime's `_div`. */
function div(a: number, b: number): number {
  return Math.floor(a / b);
}

/** The non-negative remainder, as the runtime's `_mod`. */
function mod(a: number, b: number): number {
  return a - div(a, b) * b;
}

/** Truncating division of non-negative operands, Dart's `~/`. */
function idiv(a: number, b: number): number {
  return Math.trunc(a / b);
}

const gregorian: CalendarSystem = {
  id: gregorianId,
  daysInMonth(year, month) {
    switch (month) {
      case 2:
        return year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28;
      case 4:
      case 6:
      case 9:
      case 11:
        return 30;
      case 1:
      case 3:
      case 5:
      case 7:
      case 8:
      case 10:
      case 12:
        return 31;
      default:
        return 0;
    }
  },
  eraOf: (year) => (year < 1 ? { era: 0, year: 1 - year } : { era: 1, year }),
  toJdn(d) {
    const a = idiv(14 - d.month, 12);
    const y = d.year + 4800 - a;
    const m = d.month + 12 * a - 3;
    return d.day + idiv(153 * m + 2, 5) + 365 * y + div(y, 4) - div(y, 100) + div(y, 400) - 32045;
  },
  fromJdn(jdn) {
    const a = jdn + 32044;
    const b = div(4 * a + 3, 146097);
    const c = a - div(146097 * b, 4);
    const d = div(4 * c + 3, 1461);
    const e = c - div(1461 * d, 4);
    const m = idiv(5 * e + 2, 153);
    return {
      year: 100 * b + d - 4800 + idiv(m, 10),
      month: m + 3 - 12 * idiv(m, 10),
      day: e - idiv(153 * m + 2, 5) + 1,
    };
  },
};

const ethiopicEpochOffset = 1723856;

const ethiopic: CalendarSystem = {
  id: ethiopicId,
  daysInMonth(year, month) {
    if (month >= 1 && month <= 12) {
      return 30;
    }
    return month === 13 ? (mod(year, 4) === 3 ? 6 : 5) : 0;
  },
  eraOf: (year) => (year <= 0 ? { era: 0, year: year + 5500 } : { era: 1, year }),
  toJdn: (d) => ethiopicEpochOffset + 365 * d.year + div(d.year, 4) + 30 * (d.month - 1) + d.day - 1,
  fromJdn(jdn) {
    const days = jdn - ethiopicEpochOffset;
    const c4 = div(days, 1461);
    const r = mod(days, 1461);
    const year = 4 * c4 + idiv(r, 365) - idiv(r, 1460);
    const doy = r === 1460 ? 365 : r % 365;
    return { year, month: idiv(doy, 30) + 1, day: (doy % 30) + 1 };
  },
};

const civilYearStart = (year: number): number => 354 * (year - 1) + div(3 + 11 * year, 30);
const civilLeap = (year: number): boolean => mod(14 + 11 * year, 30) < 11;
const civilDaysInMonth = (year: number, month: number): number =>
  29 + (month % 2) + (month === 12 && civilLeap(year) ? 1 : 0);

function civilFromDays(days: number): CalendarYmd {
  const year = div(30 * days + 10646, 10631);
  const ys = civilYearStart(year);
  const month = Math.min(12, Math.max(1, -div(-2 * (days - 29 - ys), 59) + 1));
  return { year, month, day: days - ys - idiv(59 * (month - 1) + 1, 2) + 1 };
}

const popcount = (n: number): number => {
  let count = 0;
  for (let bits = n; bits > 0; bits >>= 1) {
    count += bits & 1;
  }
  return count;
};

/** The first day of each Umm al-Qura year in days after the civil epoch, then the day after the table. */
const umalquraYearStarts: readonly number[] = (() => {
  const starts = [umalquraEpochDay];
  for (const mask of umalquraMonthMasks) {
    starts.push((starts[starts.length - 1] ?? 0) + 348 + popcount(mask));
  }
  return starts;
})();

const umalquraOutOfTable = (year: number): boolean => year < umalquraYearStart || year > umalquraYearEnd;

const umalqura: CalendarSystem = {
  id: islamicUmalquraId,
  daysInMonth(year, month) {
    if (month < 1 || month > 12) {
      return 0;
    }
    if (umalquraOutOfTable(year)) {
      return civilDaysInMonth(year, month);
    }
    return 29 + (((umalquraMonthMasks[year - umalquraYearStart] ?? 0) >> (12 - month)) & 1);
  },
  eraOf: (year) => (year < 1 ? { era: 1, year: 1 - year } : { era: 0, year }),
  toJdn(d) {
    if (umalquraOutOfTable(d.year)) {
      return civilEpochJdn + civilYearStart(d.year) + idiv(59 * (d.month - 1) + 1, 2) + d.day - 1;
    }
    let days = umalquraYearStarts[d.year - umalquraYearStart] ?? 0;
    for (let m = 1; m < d.month; m++) {
      days += umalqura.daysInMonth(d.year, m);
    }
    return civilEpochJdn + days + d.day - 1;
  },
  fromJdn(jdn) {
    const days = jdn - civilEpochJdn;
    if (days < (umalquraYearStarts[0] ?? 0) || days >= (umalquraYearStarts[umalquraYearStarts.length - 1] ?? 0)) {
      return civilFromDays(days);
    }
    let lo = 0;
    let hi = umalquraYearStarts.length - 1;
    while (hi - lo > 1) {
      const midpoint = idiv(lo + hi, 2);
      if ((umalquraYearStarts[midpoint] ?? 0) > days) {
        hi = midpoint;
      } else {
        lo = midpoint;
      }
    }
    const year = umalquraYearStart + lo;
    let rest = days - (umalquraYearStarts[lo] ?? 0);
    let month = 1;
    while (rest >= umalqura.daysInMonth(year, month)) {
      rest -= umalqura.daysInMonth(year, month);
      month++;
    }
    return { year, month, day: rest + 1 };
  },
};

const persianDaysBefore = [0, 31, 62, 93, 124, 155, 186, 216, 246, 276, 306, 336];

const persianNonLeap = (year: number): boolean => persianNonLeapYears.includes(year);
const persianFirstNonLeap = persianNonLeapYears[0] ?? 0;

function persianFirstDay(year: number): number {
  const n = 365 * (year - 1) + div(8 * year + 21, 33);
  return year > persianFirstNonLeap && persianNonLeap(year - 1) ? n - 1 : n;
}

function persianLeap(year: number): boolean {
  if (year >= persianFirstNonLeap && persianNonLeap(year)) {
    return false;
  }
  if (year > persianFirstNonLeap && persianNonLeap(year - 1)) {
    return true;
  }
  return mod(year * 25 + 11, 33) < 8;
}

const persian: CalendarSystem = {
  id: persianId,
  daysInMonth(year, month) {
    if (month >= 1 && month <= 6) {
      return 31;
    }
    if (month >= 7 && month <= 11) {
      return 30;
    }
    return month === 12 ? (persianLeap(year) ? 30 : 29) : 0;
  },
  eraOf: (year) => ({ era: 0, year }),
  toJdn: (d) => persianEpochJdn + persianFirstDay(d.year) + (persianDaysBefore[d.month - 1] ?? 0) + d.day - 1,
  fromJdn(jdn) {
    const days = jdn - persianEpochJdn;
    let year = div(33 * days + 3, 12053) + 1;
    let doy = days - persianFirstDay(year);
    if (doy === 365 && year >= persianFirstNonLeap && persianNonLeap(year)) {
      year++;
      doy = 0;
    }
    const month = doy < 216 ? idiv(doy, 31) + 1 : idiv(doy - 6, 30) + 1;
    return { year, month, day: doy + 1 - (persianDaysBefore[month - 1] ?? 0) };
  },
};

const systems: ReadonlyMap<string, CalendarSystem> = new Map(
  [gregorian, ethiopic, umalqura, persian].map((system) => [system.id, system]),
);

/** The system with `id`, or undefined. */
export function calendarSystem(id: string): CalendarSystem | undefined {
  return systems.get(id);
}

/** The system of an enum member name; the enum spells `islamic-umalqura` as `islamicUmalqura`. */
export function calendarSystemOf(member: unknown): CalendarSystem | undefined {
  if (typeof member !== "string") {
    return undefined;
  }
  return systems.get(member === "islamicUmalqura" ? islamicUmalquraId : member);
}

/** 0 for Sunday, as the runtime's `calendarWeekday`. */
export function calendarWeekday(jdn: number): number {
  return mod(jdn + 1, 7);
}

/** English month names as CLDR spells them; the runtime holds no other locale data for the picker. */
export const monthNames: Readonly<Record<string, readonly string[]>> = {
  [gregorianId]: [
    "January",
    "February",
    "March",
    "April",
    "May",
    "June",
    "July",
    "August",
    "September",
    "October",
    "November",
    "December",
  ],
  [ethiopicId]: [
    "Meskerem",
    "Tekemt",
    "Hedar",
    "Tahsas",
    "Ter",
    "Yekatit",
    "Megabit",
    "Miazia",
    "Genbot",
    "Sene",
    "Hamle",
    "Nehasse",
    "Pagumen",
  ],
  [islamicUmalquraId]: [
    "Muharram",
    "Safar",
    "Rabiʻ I",
    "Rabiʻ II",
    "Jumada I",
    "Jumada II",
    "Rajab",
    "Shaʻban",
    "Ramadan",
    "Shawwal",
    "Dhuʻl-Qiʻdah",
    "Dhuʻl-Hijjah",
  ],
  [persianId]: [
    "Farvardin",
    "Ordibehesht",
    "Khordad",
    "Tir",
    "Mordad",
    "Shahrivar",
    "Mehr",
    "Aban",
    "Azar",
    "Dey",
    "Bahman",
    "Esfand",
  ],
};

const eraNames: Readonly<Record<string, string>> = {
  [ethiopicId]: "AM|AA",
  [islamicUmalquraId]: "AH|BH",
  [persianId]: "AP|AP",
  [gregorianId]: "BC|AD",
};

/** The era abbreviation with a leading space; none for Gregorian years in the common era. */
export function eraSuffix(system: CalendarSystem, year: number): string {
  const { era } = system.eraOf(year);
  if (system.id === gregorianId && era === 1) {
    return "";
  }
  return ` ${(eraNames[system.id] ?? "").split("|")[era] ?? ""}`;
}

/** The month's name in `system`. */
export function monthName(system: CalendarSystem, month: number): string {
  return monthNames[system.id]?.[month - 1] ?? "";
}

/** The Julian day number of a `YYYY-MM-DD` document date, or undefined. */
export function jdnOfIsoDate(value: unknown): number | undefined {
  const match = typeof value === "string" ? /^(\d{4})-(\d{2})-(\d{2})$/.exec(value) : null;
  if (match?.[1] === undefined || match[2] === undefined || match[3] === undefined) {
    return undefined;
  }
  return gregorian.toJdn({ year: Number(match[1]), month: Number(match[2]), day: Number(match[3]) });
}

/** The secondary calendar's span over the month, such as `Shaʻban – Ramadan 1445 AH`. */
export function secondaryTitle(sec: CalendarSystem, from: number, to: number): string {
  const a = sec.fromJdn(from);
  const b = sec.fromJdn(to);
  const ea = sec.eraOf(a.year);
  const eb = sec.eraOf(b.year);
  if (a.year === b.year && a.month === b.month) {
    return `${monthName(sec, a.month)} ${ea.year}${eraSuffix(sec, a.year)}`;
  }
  if (a.year === b.year) {
    return `${monthName(sec, a.month)} – ${monthName(sec, b.month)} ${ea.year}${eraSuffix(sec, a.year)}`;
  }
  return `${monthName(sec, a.month)} ${ea.year} – ${monthName(sec, b.month)} ${eb.year}${eraSuffix(sec, b.year)}`;
}
