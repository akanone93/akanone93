// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import type { App, AppSummary } from "@plux/api";
import type { AppSort } from "./apps.schema.ts";

/** One app of the grid: the app and, for an active app, its summary (archived apps have none). */
export interface AppEntry {
  readonly app: App;
  readonly summary: AppSummary | undefined;
}

/** What the search and the filters keep (STU-023). `mine` is the apps the caller holds a grant on, or undefined for all. */
export interface AppFilter {
  readonly q: string;
  readonly team: string;
  readonly tag: string;
  readonly mine: ReadonlySet<string> | undefined;
}

/** True when `q` occurs, ignoring case, in the app's name, key, description or one of its tags. */
export function matchesQuery(app: App, q: string): boolean {
  const needle = q.trim().toLocaleLowerCase();
  if (needle === "") {
    return true;
  }
  return [app.name, app.key, app.description, ...app.tags].some((text) => text.toLocaleLowerCase().includes(needle));
}

/** The entries that pass the search, the team and tag filters and the "my apps" filter. */
export function filterApps(entries: readonly AppEntry[], filter: AppFilter): AppEntry[] {
  return entries.filter(
    ({ app }) =>
      matchesQuery(app, filter.q) &&
      (filter.team === "" || app.owningTeamId === filter.team) &&
      (filter.tag === "" || app.tags.includes(filter.tag)) &&
      (filter.mine === undefined || filter.mine.has(app.id)),
  );
}

/** The newest of an app's last publish and its last change, as epoch milliseconds; 0 when neither is known. */
export function activityOf(entry: AppEntry): number {
  const published = entry.summary?.lastPublishedAt;
  if (published !== undefined) {
    return timestampDate(published).getTime();
  }
  const updated = entry.app.updatedAt;
  return updated === undefined ? 0 : timestampDate(updated).getTime();
}

/** The crash-free rate used to order apps by health; an app without a summary or devices counts as perfectly healthy. */
function rateOf(entry: AppEntry): number {
  return entry.summary?.crashFreeRate ?? 1;
}

/**
 * Orders the entries; pinned apps always come first. By name, A to Z; by activity, newest publish first; by health,
 * lowest crash-free rate first so that apps needing attention lead. Ties fall back to the name, then the key.
 */
export function sortApps(entries: readonly AppEntry[], sort: AppSort, pinned: ReadonlySet<string>): AppEntry[] {
  const byName = (a: AppEntry, b: AppEntry): number =>
    a.app.name.localeCompare(b.app.name) || a.app.key.localeCompare(b.app.key);
  const primary = (a: AppEntry, b: AppEntry): number => {
    if (sort === "activity") {
      return activityOf(b) - activityOf(a);
    }
    if (sort === "health") {
      return rateOf(a) - rateOf(b);
    }
    return 0;
  };
  return [...entries].sort((a, b) => {
    const pin = Number(pinned.has(b.app.id)) - Number(pinned.has(a.app.id));
    return pin || primary(a, b) || byName(a, b);
  });
}

/** The tags that occur on the entries, sorted, for the tag filter. */
export function tagsOf(entries: readonly AppEntry[]): string[] {
  return [...new Set(entries.flatMap(({ app }) => app.tags))].sort((a, b) => a.localeCompare(b));
}

/** The first character of the app's name, upper case, for the monogram visual (STU-021). */
export function monogramOf(app: App): string {
  const first = Array.from(app.name.trim())[0] ?? Array.from(app.key)[0] ?? "";
  return first.toLocaleUpperCase();
}

/** Token classes of the monogram's colour; one is derived from the app's ID, so an app keeps its colour (STU-021). */
const monogramTones: readonly string[] = [
  "bg-success-surface text-success",
  "bg-warning-surface text-warning",
  "bg-selection-surface text-selection-text",
  "bg-hover text-primary",
  "bg-raised text-secondary",
];

/** The colour classes of an app's monogram: the same app always gets the same one. */
export function monogramTone(app: App): string {
  let hash = 0;
  for (const char of app.id === "" ? app.key : app.id) {
    hash = Math.imul(hash, 31) + (char.codePointAt(0) ?? 0);
  }
  return monogramTones[Math.abs(hash) % monogramTones.length] ?? "";
}

/** Crash-free rate at or above which an app is healthy, and at or above which it is merely degraded. */
export const healthyRate = 0.99;
export const degradedRate = 0.95;

export type Health =
  | { readonly kind: "none" }
  | { readonly kind: "good" | "warn" | "bad"; readonly rate: number; readonly trend: number };

/** The health of an app from its summary: no active device means no rate. */
export function healthOf(summary: AppSummary): Health {
  if (summary.activeDevices <= 0n) {
    return { kind: "none" };
  }
  const rate = summary.crashFreeRate;
  const kind = rate >= healthyRate ? "good" : rate >= degradedRate ? "warn" : "bad";
  return { kind, rate, trend: summary.crashFreeTrend };
}

export type LimitLevel = "ok" | "warning" | "over";

/** The share of the limit in force that is used, 0 to 1; a limit of zero or less is never reached. */
export function ratioOf(used: bigint, limit: bigint): number {
  return limit <= 0n ? 0 : Math.min(1, Number(used) / Number(limit));
}

/** Warning from 80 percent of the limit in force (LIM-003), over at the limit. */
export function levelOf(used: bigint, limit: bigint): LimitLevel {
  if (limit <= 0n) {
    return "ok";
  }
  if (used >= limit) {
    return "over";
  }
  return ratioOf(used, limit) >= 0.8 ? "warning" : "ok";
}

const binary = ["B", "KiB", "MiB", "GiB", "TiB"] as const;

/** The unit words a reading needs; the symbols of bytes are universal. */
export interface UnitWords {
  readonly operations: string;
  readonly codepoints: string;
  readonly microseconds: string;
  readonly milliseconds: string;
}

/** A value with its unit as the registry names it: bytes in binary units, counts plain, the rest with their word. */
export function formatAmount(value: bigint, unit: string, locale: string, words: UnitWords): string {
  const number = (n: number, digits: number) =>
    new Intl.NumberFormat(locale, { maximumFractionDigits: digits }).format(n);
  if (unit === "bytes") {
    let scaled = Number(value);
    let step = 0;
    while (scaled >= 1024 && step < binary.length - 1) {
      scaled /= 1024;
      step += 1;
    }
    return `${number(scaled, step === 0 ? 0 : 1)} ${binary[step]}`;
  }
  const plain = number(Number(value), 0);
  const word: Readonly<Record<string, string>> = {
    operations: words.operations,
    codepoints: words.codepoints,
    microseconds: words.microseconds,
    milliseconds: words.milliseconds,
  };
  const suffix = word[unit];
  return suffix === undefined ? plain : `${plain} ${suffix}`;
}
