// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * `CupertinoDatePicker` of the layout conformance scenes (STU-005, S4.4): the
 * `time`, `date` and `dateAndTime` modes initialised to 9 April 2018 10:30
 * (the Flutter side passes the same `initialDateTime`), under the English
 * `DefaultCupertinoLocalizations` the golden writer installs. The picker
 * lays text out unscaled (`MediaQuery.withNoTextScaling`).
 */

import { cupertinoDatePicker, kDatePickerItemExtent, type RenderBox, TextDirection, TextScaler } from "../src/index.ts";
import { type CupertinoContext, label, numberProp, str } from "./cupertino-common.ts";
import type { SceneNode } from "./scene.ts";

/** The node types of this file. */
export const cupertinoDatePickerTypes: ReadonlySet<string> = new Set(["CupertinoDatePicker"]);

const shortWeekdays = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
const shortMonths = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const months = [
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
];

const initial = { year: 2018, month: 4, day: 9, hour: 10, minute: 30 };

/** `datePickerMediumDate`. */
function mediumDate(year: number, month: number, day: number): string {
  const weekday = (new Date(Date.UTC(year, month - 1, day)).getUTCDay() + 6) % 7;
  return `${shortWeekdays[weekday]} ${shortMonths[month - 1]} ${String(day).padEnd(2)}`;
}

const range = (count: number, from = 0): number[] => Array.from({ length: count }, (_, i) => from + i);

/** The rows of a column from its selected index on; a looping column continues round to the rows before it. */
function fromSelected<T>(items: readonly T[], selected: number, looping: boolean): T[] {
  return looping ? [...items.slice(selected), ...items.slice(0, selected)] : items.slice(selected);
}

interface Column {
  /** The texts the column's width is measured over. */
  widest: string[];
  /** The texts of the rows, from the selected one. */
  rows: string[];
}

const pad2 = (value: number): string => String(value).padStart(2, "0");

function hourColumn(use24h: boolean): Column {
  const texts = range(24).map((hour) => String(use24h ? hour : ((hour + 11) % 12) + 1));
  return { widest: range(24).map(String), rows: fromSelected(texts, initial.hour, true) };
}

const minuteColumn = (): Column => ({
  widest: range(60).map(pad2),
  rows: fromSelected(range(60).map(pad2), initial.minute, true),
});

const dayPeriodColumn = (): Column => ({ widest: ["AM", "PM"], rows: ["AM", "PM"] });

const dateColumn = (): Column => ({
  widest: range(12, 1).map((month) => mediumDate(2018, month, 25)),
  rows: range(10).map((offset) => mediumDate(initial.year, initial.month, initial.day + offset)),
});

const monthColumn = (): Column => ({ widest: months, rows: fromSelected(months, initial.month - 1, false) });

const dayColumn = (): Column => ({
  widest: range(31, 1).map(String),
  rows: range(30, 1)
    .slice(initial.day - 1)
    .map(String),
});

const yearColumn = (): Column => ({
  widest: [String(initial.year)],
  rows: range(10).map((offset) => String(initial.year + offset)),
});

/** The columns in the order Flutter builds the pickers, and in the order of its column widths. */
function columnsFor(mode: string, use24h: boolean, rtl: boolean): { pickers: Column[]; widths: Column[] } {
  if (mode === "date") {
    const columns = [monthColumn(), dayColumn(), yearColumn()];
    return { pickers: columns, widths: columns };
  }
  const time = [hourColumn(use24h), minuteColumn()];
  const pickers = rtl ? [time[1] as Column, time[0] as Column] : [...time];
  const widths = [...time];
  if (!use24h) {
    pickers.push(dayPeriodColumn());
    widths.push(dayPeriodColumn());
  }
  if (mode === "dateAndTime") {
    pickers.unshift(dateColumn());
    widths.unshift(dateColumn());
  }
  return { pickers, widths };
}

/** Builds a `CupertinoDatePicker`; null when `node` is not one. */
export function buildCupertinoDatePicker(node: SceneNode, ctx: CupertinoContext): RenderBox | null {
  if (node.type !== "CupertinoDatePicker") {
    return null;
  }
  const mode = str(node, "mode", "dateAndTime");
  const text = ctx.text === null ? null : { ...ctx.text, textScaler: TextScaler.noScaling };
  const unscaled: CupertinoContext = { ...ctx, text };
  const row = (value: string): RenderBox => label(unscaled, value, { fontSize: 21 });
  const { pickers, widths } = columnsFor(
    mode,
    node.props["use24hFormat"] === true,
    ctx.textDirection === TextDirection.rtl,
  );
  const picker = cupertinoDatePicker({
    kind: mode === "date" ? "date" : "dateTime",
    pickers: pickers.map((column) => column.rows.map(row)),
    columnWidths: widths.map((column) =>
      Math.max(...column.widest.map((value) => row(value).getMaxIntrinsicWidth(Number.POSITIVE_INFINITY))),
    ),
    itemExtent: numberProp(node, "itemExtent", kDatePickerItemExtent),
    textDirection: ctx.textDirection,
  });
  picker.debugId = node.id;
  return picker;
}
