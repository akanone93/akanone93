// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";
import { limitDefault } from "@plux/schema";

export type NotificationKind = "work" | "approvals" | "operations" | "security";

export const kindOrder: readonly NotificationKind[] = ["work", "approvals", "operations", "security"];

const operationsPrefixes = ["rollout.", "alert.", "function.", "licence.", "killswitch.", "limit."];
const securityPrefixes = ["security.", "access.", "device.", "session."];

/** F16: the kind of an event type, from its dotted prefix; unknown types count as work. */
export function kindOf(eventType: string): NotificationKind {
  if (eventType.startsWith("approval.") || eventType.startsWith("breakglass.")) {
    return "approvals";
  }
  if (securityPrefixes.some((p) => eventType.startsWith(p))) {
    return "security";
  }
  return operationsPrefixes.some((p) => eventType.startsWith(p)) ? "operations" : "work";
}

/** Approval and security notifications cannot be muted (F16). */
export function isMutable(eventType: string): boolean {
  const kind = kindOf(eventType);
  return kind !== "approvals" && kind !== "security";
}

/** Days an inbox item is kept, from the limits registry. */
export function retentionDays(): number {
  return limitDefault("notifications.retentionDays");
}

const mutedKey = "plux.notifications.muted";

export function readMuted(): ReadonlySet<string> {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(mutedKey) ?? "[]");
    return new Set(Array.isArray(parsed) ? parsed.filter((v): v is string => typeof v === "string") : []);
  } catch {
    return new Set();
  }
}

export function writeMuted(muted: ReadonlySet<string>): void {
  try {
    localStorage.setItem(mutedKey, JSON.stringify([...muted]));
  } catch {
    // Mutes are a per-device convenience; a blocked store keeps the defaults.
  }
}

export interface InboxLike {
  readonly id: string;
  readonly eventType: string;
  readonly appId: string;
  readonly read: boolean;
  readonly link: string;
  readonly groupKey: string;
  readonly groupCount: number;
  readonly params: Readonly<Record<string, string>>;
}

export interface InboxGroup<T extends InboxLike> {
  readonly key: string;
  readonly eventType: string;
  readonly items: readonly T[];
  readonly count: number;
  readonly unread: boolean;
}

/** Repeats of one type (and server group key) collapse to a single row with a count. */
export function groupItems<T extends InboxLike>(items: readonly T[]): InboxGroup<T>[] {
  const groups = new Map<string, T[]>();
  for (const item of items) {
    const key = `${item.eventType}|${item.groupKey}`;
    groups.set(key, [...(groups.get(key) ?? []), item]);
  }
  return [...groups].map(([key, members]) => ({
    key,
    eventType: members[0]?.eventType ?? "",
    items: members,
    count: members.reduce((n, m) => n + Math.max(m.groupCount, 1), 0),
    unread: members.some((m) => !m.read),
  }));
}

type Render = (i18n: I18n, count: number) => string;

const comment = "Grouped notification line; count is how many events of this type were grouped.";

const renderers: Readonly<Record<string, Render>> = {
  "release.published": (i18n, count) =>
    i18n._({
      id: "notify.type.release.published",
      message: "{count, plural, one {# publish finished} other {# publishes finished}}",
      comment,
      values: { count },
    }),
  "lock.requested": (i18n, count) =>
    i18n._({
      id: "notify.type.lock.requested",
      message: "{count, plural, one {# lock requested} other {# locks requested}}",
      comment,
      values: { count },
    }),
  "approval.requested": (i18n, count) =>
    i18n._({
      id: "notify.type.approval.requested",
      message: "{count, plural, one {# approval requested} other {# approvals requested}}",
      comment,
      values: { count },
    }),
  "rollout.paused": (i18n, count) =>
    i18n._({
      id: "notify.type.rollout.paused",
      message: "{count, plural, one {# rollout paused by a health gate} other {# rollouts paused by a health gate}}",
      comment,
      values: { count },
    }),
  "function.failing": (i18n, count) =>
    i18n._({
      id: "notify.type.function.failing",
      message: "{count, plural, one {# function failing} other {# functions failing}}",
      comment,
      values: { count },
    }),
  "alert.fired": (i18n, count) =>
    i18n._({
      id: "notify.type.alert.fired",
      message: "{count, plural, one {# alert fired} other {# alerts fired}}",
      comment,
      values: { count },
    }),
  "breakglass.used": (i18n, count) =>
    i18n._({
      id: "notify.type.breakglass.used",
      message: "{count, plural, one {# break-glass use} other {# break-glass uses}}",
      comment,
      values: { count },
    }),
  "breakglass.review_overdue": (i18n, count) =>
    i18n._({
      id: "notify.type.breakglass.review_overdue",
      message: "{count, plural, one {# break-glass review overdue} other {# break-glass reviews overdue}}",
      comment,
      values: { count },
    }),
  "licence.expiring": (i18n, count) =>
    i18n._({
      id: "notify.type.licence.expiring",
      message: "{count, plural, one {# licence expiring} other {# licences expiring}}",
      comment,
      values: { count },
    }),
};

/** The event types this Studio can word; the profile lists them for muting. */
export const knownTypes: readonly string[] = Object.keys(renderers);

/** Text rendered at display time from the type and its typed parameters; never stored text (O-18). */
export function describe(i18n: I18n, item: InboxLike, count: number): string {
  const render = renderers[item.eventType];
  if (render !== undefined) {
    return render(i18n, count);
  }
  return i18n._({
    id: "notify.type.unknown",
    message: "{count, plural, one {# notification} other {# notifications}}",
    comment: "Fallback line for a notification type this Studio version does not know.",
    values: { count },
  });
}
