// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { timestampFromMs } from "@bufbuild/protobuf/wkt";
import type { AuditEntry } from "@plux/api";
import { auditCsv, keepEntry, serverFilter } from "./audit-model.ts";

const entry = (action: string, display: string, ms: number) =>
  ({
    id: "1",
    action,
    targetKind: "role",
    targetId: "qa",
    requestId: "r",
    actor: { display, id: "u" },
    time: timestampFromMs(ms),
  }) as unknown as AuditEntry;

describe("[STU-116] audit export", () => {
  test("the server filter holds only equalities and never a quote from the input", () => {
    expect(serverFilter("role.create", 'a"b')).toBe('action = "role.create" AND target_id = "ab"');
    expect(serverFilter(undefined, "")).toBe("");
  });

  test("the date range is inclusive of whole days and the actor is matched case-insensitively", () => {
    const e = entry("x", "Grace", Date.UTC(2026, 0, 6, 23));
    expect(keepEntry(e, "2026-01-06", "2026-01-06", "grace")).toBe(true);
    expect(keepEntry(e, "2026-01-07", undefined, undefined)).toBe(false);
    expect(keepEntry(e, undefined, "2026-01-05", undefined)).toBe(false);
    expect(keepEntry(e, undefined, undefined, "ada")).toBe(false);
  });

  test("the CSV quotes cells and neutralises formulas", () => {
    const csv = auditCsv([entry("=HYPERLINK(1)", 'Gra"ce', Date.UTC(2026, 0, 6))]);
    const [head, row] = csv.split("\r\n");
    expect(head).toBe("time,actor,action,resource,request");
    expect(row).toBe('"2026-01-06T00:00:00.000Z","Gra""ce","\'=HYPERLINK(1)","role qa","r"');
  });
});
