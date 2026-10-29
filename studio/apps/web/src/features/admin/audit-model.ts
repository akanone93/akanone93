// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import type { AuditEntry } from "@plux/api";

/** The server-side filter of the audit list: equalities on action and target (the filter language has no others). */
export function serverFilter(action: string | undefined, resource: string | undefined): string {
  const clause = (key: string, value: string | undefined) =>
    value === undefined || value === "" ? [] : [`${key} = "${value.replaceAll('"', "")}"`];
  return [...clause("action", action), ...clause("target_id", resource)].join(" AND ");
}

/** Whether an entry lies in the date range (inclusive, whole days) and names the actor; the server cannot filter these. */
export function keepEntry(
  e: Pick<AuditEntry, "time" | "actor">,
  from: string | undefined,
  to: string | undefined,
  actor: string | undefined,
): boolean {
  const at = e.time === undefined ? undefined : timestampDate(e.time).getTime();
  const start = from === undefined || from === "" ? undefined : Date.parse(from);
  const end = to === undefined || to === "" ? undefined : Date.parse(to) + 86_400_000;
  if (at !== undefined && ((start !== undefined && at < start) || (end !== undefined && at >= end))) {
    return false;
  }
  const who = `${e.actor?.display ?? ""} ${e.actor?.id ?? ""}`.toLowerCase();
  return actor === undefined || actor === "" || who.includes(actor.toLowerCase());
}

/** One CSV cell; a leading formula character is neutralised so a spreadsheet never runs an entry's text. */
function cell(value: string): string {
  const safe = /^[=+\-@\t\r]/.test(value) ? `'${value}` : value;
  return `"${safe.replaceAll('"', '""')}"`;
}

/** The entries as CSV with a header row. */
export function auditCsv(entries: readonly AuditEntry[]): string {
  const rows = entries.map((e) =>
    [
      e.time === undefined ? "" : timestampDate(e.time).toISOString(),
      e.actor?.display || e.actor?.id || "",
      e.action,
      `${e.targetKind} ${e.targetId}`.trim(),
      e.requestId,
    ]
      .map(cell)
      .join(","),
  );
  return [["time", "actor", "action", "resource", "request"].join(","), ...rows].join("\r\n");
}
