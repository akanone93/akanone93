// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * `CupertinoDatePicker` of the layout conformance scenes (STU-005, S4.4): the
 * `time`, `date` and `dateAndTime` modes initialised to 9 April 2018 10:30
 * (the Flutter side passes the same `initialDateTime`), under the English
 * `DefaultCupertinoLocalizations` the golden writer installs. The picker
 * lays text out unscaled (`MediaQuery.withNoTextScaling`).
 */

import { cupertinoDatePicker, kDatePickerItemExtent, type RenderBox, TextDirection, TextScaler } from "../index.ts";
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

/** `datePickerMediumDate` of the initial day plus `offset` days. */
function mediumDate(year: number, month: number, day: number): string {
  const date = new Date(Date.UTC(year, month - 1, day));
  const weekday = (date.getUTCDay() + 6) % 7;
  return `${shortWeekdays[weekday]} ${shortMonths[date.getUTCMonth()]} ${String(date.getUTCDate()).padEnd(2)}`;
}

const range = (count: number, from = 0): number[] => Array.from({ length: count }, (_, i) => from + i);

/** `x % n` as Dart computes it: never negative. */
const wrap = (index: number, count: number): number => ((index % count) + count) % count;

interface Column {
  /** The texts the column's width is measured over. */
  widest: string[];
  /** The selected item (`initialItem` of the column's controller). */
  selected: number;
  /** Whether the column builds a row at an item index. */
  exists: (index: number) => boolean;
  /** The text of the row at an item index. */
  text: (index: number) => string;
}

const pad2 = (value: number): string => String(value).padStart(2, "0");

/** A `looping: true` column over `texts`. */
function looping(texts: readonly string[], selected: number, widest: readonly string[] = texts): Column {
  return {
    widest: [...widest],
    selected,
    exists: () => true,
    text: (index) => texts[wrap(index, texts.length)] ?? "",
  };
}

function hourColumn(use24h: boolean): Column {
  const texts = range(24).map((hour) => String(use24h ? hour : ((hour + 11) % 12) + 1));
  return looping(texts, initial.hour, range(24).map(String));
}

const minuteColumn = (): Column => looping(range(60).map(pad2), initial.minute);

const dayPeriodColumn = (): Column => ({
  widest: ["AM", "PM"],
  selected: 0,
  exists: (index) => index === 0 || index === 1,
  text: (index) => (index === 0 ? "AM" : "PM"),
});

const dateColumn = (): Column => ({
  widest: range(12, 1).map((month) => mediumDate(2018, month, 25)),
  selected: 0,
  exists: () => true,
  text: (index) => mediumDate(initial.year, initial.month, initial.day + index),
});

const monthColumn = (): Column => looping(months, initial.month - 1);

const dayColumn = (): Column => looping(range(31, 1).map(String), initial.day - 1);

const yearColumn = (): Column => ({
  widest: [String(initial.year)],
  selected: initial.year,
  exists: (index) => index >= 1,
  text: String,
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
    pickers: pickers.map((column) => ({
      exists: column.exists,
      build: (index: number) => row(column.text(index)),
      selectedItem: column.selected,
    })),
    rowId: (column, index) => `${node.id}.c${column}.i${index}`,
    columnWidths: widths.map((column) =>
      Math.max(...column.widest.map((value) => row(value).getMaxIntrinsicWidth(Number.POSITIVE_INFINITY))),
    ),
    itemExtent: numberProp(node, "itemExtent", kDatePickerItemExtent),
    textDirection: ctx.textDirection,
  });
  picker.debugId = node.id;
  return picker;
}
