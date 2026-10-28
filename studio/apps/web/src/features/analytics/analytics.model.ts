// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import type { RollupPoint } from "@plux/api";

export interface Total {
  readonly key: string;
  readonly value: number;
}

const dayOf = (p: RollupPoint): string =>
  p.bucket === undefined ? "" : timestampDate(p.bucket).toISOString().slice(0, 10);

/** Events per UTC day, oldest first. */
export function dailyEvents(points: readonly RollupPoint[]): Total[] {
  const days = new Map<string, number>();
  for (const p of points) {
    days.set(dayOf(p), (days.get(dayOf(p)) ?? 0) + Number(p.events));
  }
  return [...days].sort(([a], [b]) => a.localeCompare(b)).map(([key, value]) => ({ key, value }));
}

/** Events summed by one dimension, largest first; ties keep key order. */
export function totalsBy(points: readonly RollupPoint[], pick: (p: RollupPoint) => string): Total[] {
  const sums = new Map<string, number>();
  for (const p of points) {
    sums.set(pick(p), (sums.get(pick(p)) ?? 0) + Number(p.events));
  }
  return [...sums]
    .map(([key, value]) => ({ key, value }))
    .sort((a, b) => b.value - a.value || a.key.localeCompare(b.key));
}

export interface Percentiles {
  readonly days: readonly string[];
  readonly p50: readonly number[];
  readonly p90: readonly number[];
  readonly p99: readonly number[];
}

/** The percentiles per day; where several series share a day the worst one is shown. */
export function percentilesByDay(points: readonly RollupPoint[]): Percentiles {
  const days = new Map<string, [number, number, number]>();
  for (const p of points) {
    const cur = days.get(dayOf(p)) ?? [0, 0, 0];
    days.set(dayOf(p), [Math.max(cur[0], p.p50), Math.max(cur[1], p.p90), Math.max(cur[2], p.p99)]);
  }
  const rows = [...days].sort(([a], [b]) => a.localeCompare(b));
  return {
    days: rows.map(([d]) => d),
    p50: rows.map(([, v]) => v[0]),
    p90: rows.map(([, v]) => v[1]),
    p99: rows.map(([, v]) => v[2]),
  };
}
