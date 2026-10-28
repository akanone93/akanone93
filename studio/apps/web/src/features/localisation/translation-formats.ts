// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Translation interchange formats (I18N-009). Every file is untrusted: it is parsed as data and its text is never interpreted as markup. */
export type TranslationFormat = "xliff" | "arb" | "json" | "csv";

export const FORMATS: readonly TranslationFormat[] = ["xliff", "arb", "json", "csv"];

/** Messages by translation key name (not identifier). */
export type Entries = Record<string, string>;

export type ParseResult =
  | { readonly ok: true; readonly entries: Entries }
  | { readonly ok: false; readonly reason: "tooLarge" | "malformed" | "empty" };

const malformed: ParseResult = { ok: false, reason: "malformed" };

/** The format a file name suggests, if any. */
export function formatOfName(name: string): TranslationFormat | undefined {
  const ext = name.slice(name.lastIndexOf(".") + 1).toLowerCase();
  const byExt: Record<string, TranslationFormat> = {
    xlf: "xliff",
    xliff: "xliff",
    arb: "arb",
    json: "json",
    csv: "csv",
  };
  return byExt[ext];
}

const isRecord = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

/** A flat JSON object of strings; ARB also carries `@`-prefixed metadata, which is dropped. */
function parseFlat(text: string, arb: boolean): ParseResult {
  let value: unknown;
  try {
    value = JSON.parse(text);
  } catch {
    return malformed;
  }
  if (!isRecord(value)) {
    return malformed;
  }
  const entries: Entries = Object.create(null) as Entries;
  for (const [k, v] of Object.entries(value)) {
    if (arb && k.startsWith("@")) {
      continue;
    }
    if (typeof v !== "string") {
      return malformed;
    }
    entries[k] = v;
  }
  return { ok: true, entries };
}

interface CsvState {
  rows: string[][];
  row: string[];
  cell: string;
  quoted: boolean;
}

/** One character inside quotes; returns how many characters it consumed. */
function inQuotes(s: CsvState, text: string, i: number): number {
  const c = text.charAt(i);
  if (c !== '"') {
    s.cell += c;
    return 1;
  }
  if (text.charAt(i + 1) === '"') {
    s.cell += '"';
    return 2;
  }
  s.quoted = false;
  return 1;
}

function endRow(s: CsvState): void {
  s.row.push(s.cell);
  s.rows.push(s.row);
  s.row = [];
  s.cell = "";
}

/** One character outside quotes; returns how many characters it consumed. */
function outOfQuotes(s: CsvState, text: string, i: number): number {
  const c = text.charAt(i);
  if (c === '"' && s.cell === "") {
    s.quoted = true;
  } else if (c === ",") {
    s.row.push(s.cell);
    s.cell = "";
  } else if (c === "\n" || c === "\r") {
    endRow(s);
    return c === "\r" && text.charAt(i + 1) === "\n" ? 2 : 1;
  } else {
    s.cell += c;
  }
  return 1;
}

/** RFC 4180 records; a cell leading with an apostrophe before a formula character is unescaped (see `csvCell`). */
export function csvRecords(text: string): string[][] | undefined {
  const s: CsvState = { rows: [], row: [], cell: "", quoted: false };
  for (let i = 0; i < text.length; ) {
    i += s.quoted ? inQuotes(s, text, i) : outOfQuotes(s, text, i);
  }
  if (s.quoted) {
    return undefined;
  }
  if (s.cell !== "" || s.row.length > 0) {
    endRow(s);
  }
  return s.rows.filter((r) => r.some((c) => c !== ""));
}

const formula = /^'[=+\-@\t\r]/;

function parseCsv(text: string): ParseResult {
  const rows = csvRecords(text);
  if (rows === undefined) {
    return malformed;
  }
  const body = rows[0]?.[0]?.toLowerCase() === "key" ? rows.slice(1) : rows;
  const entries: Entries = Object.create(null) as Entries;
  for (const r of body) {
    const [key, value] = r;
    if (r.length < 2 || key === undefined || value === undefined || key === "") {
      return malformed;
    }
    entries[key] = formula.test(value) ? value.slice(1) : value;
  }
  return { ok: true, entries };
}

/** The descendants with a local name, whatever their namespace prefix. */
function named(root: Document | Element, name: string): Element[] {
  return Array.from(root.getElementsByTagName("*")).filter((e) => e.localName === name);
}

function parseXliff(text: string): ParseResult {
  const doc = new DOMParser().parseFromString(text, "application/xml");
  if (doc.getElementsByTagName("parsererror").length > 0 || doc.documentElement.localName !== "xliff") {
    return malformed;
  }
  const entries: Entries = Object.create(null) as Entries;
  for (const unit of named(doc, "unit")) {
    const id = unit.getAttribute("id") ?? "";
    const target = named(unit, "target")[0];
    if (id === "") {
      return malformed;
    }
    if (target !== undefined) {
      entries[id] = target.textContent ?? "";
    }
  }
  return { ok: true, entries };
}

/** Parses one import file. `maxBytes` caps its size (`translation.importSize`). */
export function parseTranslations(format: TranslationFormat, text: string, maxBytes: number): ParseResult {
  if (new TextEncoder().encode(text).length > maxBytes) {
    return { ok: false, reason: "tooLarge" };
  }
  const result =
    format === "xliff" ? parseXliff(text) : format === "csv" ? parseCsv(text) : parseFlat(text, format === "arb");
  return result.ok && Object.keys(result.entries).length === 0 ? { ok: false, reason: "empty" } : result;
}

const xmlEscapes: Record<string, string> = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" };
const xml = (s: string): string => s.replace(/[&<>"]/g, (c) => xmlEscapes[c] ?? c);

/** A CSV cell, quoted when it needs it and defanged against spreadsheet formulas. */
export function csvCell(value: string): string {
  const safe = /^[=+\-@\t\r]/.test(value) || formula.test(value) ? `'${value}` : value;
  return /[",\n\r]/.test(safe) ? `"${safe.replace(/"/g, '""')}"` : safe;
}

export interface ExportInput {
  readonly locale: string;
  readonly sourceLocale: string;
  readonly keys: readonly string[];
  readonly source: Readonly<Record<string, string>>;
  readonly target: Readonly<Record<string, string>>;
}

function toXliff(x: ExportInput): string {
  const units = x.keys.map(
    (k) =>
      `    <unit id="${xml(k)}"><segment><source>${xml(x.source[k] ?? "")}</source><target>${xml(x.target[k] ?? "")}</target></segment></unit>`,
  );
  return `<?xml version="1.0" encoding="UTF-8"?>\n<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="${xml(x.sourceLocale)}" trgLang="${xml(x.locale)}">\n  <file id="messages">\n${units.join("\n")}\n  </file>\n</xliff>\n`;
}

/** The text of an export file, in key order, with every key listed (an untranslated key has an empty text). */
export function exportTranslations(format: TranslationFormat, x: ExportInput): string {
  const pairs = x.keys.map((k) => [k, x.target[k] ?? ""] as const);
  switch (format) {
    case "xliff":
      return toXliff(x);
    case "csv":
      return `${["key", x.locale].join(",")}\n${pairs.map(([k, v]) => `${csvCell(k)},${csvCell(v)}`).join("\n")}\n`;
    case "arb":
      return `${JSON.stringify({ "@@locale": x.locale, ...Object.fromEntries(pairs) }, null, 2)}\n`;
    case "json":
      return `${JSON.stringify(Object.fromEntries(pairs), null, 2)}\n`;
  }
}
