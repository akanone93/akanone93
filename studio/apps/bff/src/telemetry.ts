// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** One browser event: names and codes only, never free text or stacks (OBS-001). */
export type BrowserEvent = {
  name: string;
  kind: "page" | "timing" | "error";
  code?: string;
  durationMs?: number;
  route?: string;
};

const ident = /^[a-z][a-z0-9_.-]{0,63}$/;
const route = /^\/[A-Za-z0-9_/.:-]{0,127}$/;
export const maxBatch = 50;
export const maxTelemetryBytes = 65536;

type Raw = { name?: unknown; kind?: unknown; code?: unknown; durationMs?: unknown; route?: unknown };
const fields = ["name", "kind", "code", "durationMs", "route"];
const kinds = ["page", "timing", "error"];

function matches(v: unknown, re: RegExp): v is string {
  return typeof v === "string" && re.test(v);
}

function validDuration(v: unknown): v is number {
  return typeof v === "number" && Number.isFinite(v) && v >= 0 && v <= 3600000;
}

function parseEvent(e: unknown): BrowserEvent | undefined {
  if (typeof e !== "object" || e === null || Object.keys(e).some((k) => !fields.includes(k))) {
    return undefined;
  }
  const r = e as Raw;
  if (!matches(r.name, ident) || typeof r.kind !== "string" || !kinds.includes(r.kind)) {
    return undefined;
  }
  const ev: BrowserEvent = { name: r.name, kind: r.kind as BrowserEvent["kind"] };
  if (r.code !== undefined && !matches(r.code, ident)) {
    return undefined;
  }
  if (r.durationMs !== undefined && !validDuration(r.durationMs)) {
    return undefined;
  }
  if (r.route !== undefined && !matches(r.route, route)) {
    return undefined;
  }
  if (r.code !== undefined) {
    ev.code = r.code as string;
  }
  if (r.durationMs !== undefined) {
    ev.durationMs = r.durationMs as number;
  }
  if (r.route !== undefined) {
    ev.route = r.route as string;
  }
  return ev;
}

/** Validates a batch; returns undefined when any part does not match the schema. */
export function parseBatch(raw: unknown): BrowserEvent[] | undefined {
  const events = typeof raw === "object" && raw !== null ? (raw as { events?: unknown }).events : undefined;
  if (!Array.isArray(events) || events.length === 0 || events.length > maxBatch) {
    return undefined;
  }
  const out: BrowserEvent[] = [];
  for (const e of events as unknown[]) {
    const ev = parseEvent(e);
    if (ev === undefined) {
      return undefined;
    }
    out.push(ev);
  }
  return out;
}

/** Structured JSON line to stdout; carries no cookies and no addresses. */
export function stdoutLine(line: Record<string, unknown>): void {
  process.stdout.write(`${JSON.stringify(line)}\n`);
}
