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

/** The local part of a qualified XML name: `xliff:unit` is `unit`. */
function localName(qualified: string): string {
  return qualified.slice(qualified.indexOf(":") + 1);
}

const xmlEntities: Readonly<Record<string, string>> = { lt: "<", gt: ">", amp: "&", quot: '"', apos: "'" };

/** Replaces XML's predefined and numeric character references; undefined for any other reference. */
function unescapeXml(text: string): string | undefined {
  let bad = false;
  const out = text.replace(/&([^;&\s<]*)(;?)/g, (_m, name: string, end: string) => {
    if (end === "") {
      bad = true;
      return "";
    }
    const numeric = /^#(?:x([0-9a-fA-F]+)|([0-9]+))$/.exec(name);
    const code = numeric ? Number.parseInt(numeric[1] ?? numeric[2] ?? "", numeric[1] ? 16 : 10) : Number.NaN;
    if (numeric && code <= 0x10ffff) {
      return String.fromCodePoint(code);
    }
    const named = xmlEntities[name];
    if (named === undefined) {
      bad = true;
      return "";
    }
    return named;
  });
  return bad ? undefined : out;
}

const xmlAttribute = /([^\s=/>]+)\s*=\s*(?:"([^"<]*)"|'([^'<]*)')/g;

/** One event of an XML document: text (references replaced), a start tag, or an end tag. */
type XmlEvent =
  | { readonly kind: "text"; readonly text: string }
  | {
      readonly kind: "start";
      readonly name: string;
      readonly attributes: ReadonlyMap<string, string>;
      readonly empty: boolean;
    }
  | { readonly kind: "end"; readonly name: string };

/** The attributes of a start tag by local name; undefined when a value holds a bad reference. */
function xmlAttributes(source: string): Map<string, string> | undefined {
  const attributes = new Map<string, string>();
  for (const m of source.matchAll(xmlAttribute)) {
    const value = unescapeXml(m[2] ?? m[3] ?? "");
    if (value === undefined) {
      return undefined;
    }
    attributes.set(localName(m[1] ?? ""), value);
  }
  return attributes;
}

/** The event of one tag's source between `<` and `>`; undefined when it is not a well-formed tag. */
function xmlTag(tag: string): XmlEvent | undefined {
  const closing = tag.startsWith("/");
  const empty = !closing && tag.endsWith("/");
  const body = tag.slice(closing ? 1 : 0, empty ? -1 : undefined);
  const name = /^[^\s/>]+/.exec(body)?.[0] ?? "";
  if (name === "") {
    return undefined;
  }
  if (closing) {
    return { kind: "end", name };
  }
  const attributes = xmlAttributes(body.slice(name.length));
  return attributes === undefined ? undefined : { kind: "start", name, attributes, empty };
}

/** The markup at `lt`: its events and where the text after it starts; undefined when it is malformed or refused. */
function xmlMarkup(text: string, lt: number): { events: XmlEvent[]; next: number } | undefined {
  const until = (open: string, close: string): number => (text.startsWith(open, lt) ? text.indexOf(close, lt) : -2);
  for (const [open, close] of [
    ["<!--", "-->"],
    ["<?", "?>"],
  ] as const) {
    const end = until(open, close);
    if (end !== -2) {
      return end < 0 ? undefined : { events: [], next: end + close.length };
    }
  }
  const cdata = until("<![CDATA[", "]]>");
  if (cdata !== -2) {
    const raw = text.slice(lt + "<![CDATA[".length, cdata);
    return cdata < 0 ? undefined : { events: [{ kind: "text", text: raw }], next: cdata + 3 };
  }
  const end = text.indexOf(">", lt);
  // A document type (or any other declaration) is refused, so no entity is ever defined.
  const event = end < 0 || text.startsWith("<!", lt) ? undefined : xmlTag(text.slice(lt + 1, end));
  return event === undefined ? undefined : { events: [event], next: end + 1 };
}

/**
 * The events of a small XML document: well-formed elements, comments, processing instructions, CDATA and
 * character references. Undefined when it is not well formed or declares a document type.
 */
function xmlEvents(text: string): XmlEvent[] | undefined {
  const events: XmlEvent[] = [];
  let at = 0;
  while (at < text.length) {
    const lt = text.indexOf("<", at);
    const raw = unescapeXml(text.slice(at, lt < 0 ? text.length : lt));
    if (raw === undefined) {
      return undefined;
    }
    events.push({ kind: "text", text: raw });
    if (lt < 0) {
      break;
    }
    const markup = xmlMarkup(text, lt);
    if (markup === undefined) {
      return undefined;
    }
    events.push(...markup.events);
    at = markup.next;
  }
  return events;
}

/** Follows the nesting of XML events: the open elements and the number of root elements. */
class XmlNesting {
  readonly open: string[] = [];
  roots = 0;

  /** Takes one event; false when it breaks the nesting under one root named `root`. */
  take(e: XmlEvent, root: string): boolean {
    if (e.kind === "text") {
      return this.open.length > 0 || e.text.trim() === "";
    }
    if (e.kind === "end") {
      return this.open.pop() === e.name;
    }
    if (this.open.length === 0) {
      this.roots += 1;
      if (localName(e.name) !== root) {
        return false;
      }
    }
    if (!e.empty) {
      this.open.push(e.name);
    }
    return true;
  }
}

/** Whether the events nest properly under one root element named `root`, with nothing but space outside it. */
function wellNested(events: readonly XmlEvent[], root: string): boolean {
  const nesting = new XmlNesting();
  return events.every((e) => nesting.take(e, root)) && nesting.roots === 1 && nesting.open.length === 0;
}

/** Collects the units of an XLIFF document from its events: each unit's id and its first target's text. */
class XliffUnits {
  readonly entries: Entries = Object.create(null) as Entries;
  private unit: string | undefined;
  private depth = 0;
  private target: { depth: number; text: string } | undefined;

  /** Takes one event; false when a unit has no id. */
  take(e: XmlEvent): boolean {
    if (e.kind === "text") {
      if (this.target !== undefined) {
        this.target.text += e.text;
      }
      return true;
    }
    if (e.kind === "end") {
      this.end();
      return true;
    }
    this.depth += e.empty ? 0 : 1;
    return this.target !== undefined || this.start(localName(e.name), e);
  }

  private end(): void {
    this.depth -= 1;
    if (this.target !== undefined && this.depth < this.target.depth) {
      this.entries[this.unit ?? ""] = this.target.text;
      this.target = undefined;
    }
  }

  private start(name: string, e: Extract<XmlEvent, { kind: "start" }>): boolean {
    if (name === "unit") {
      this.unit = e.attributes.get("id") ?? "";
      return this.unit !== "";
    }
    if (name === "target" && this.unit !== undefined && !(this.unit in this.entries)) {
      if (e.empty) {
        this.entries[this.unit] = "";
      } else {
        this.target = { depth: this.depth, text: "" };
      }
    }
    return true;
  }
}

/**
 * Reads an XLIFF document with a strict reader of the little XML it needs (`xmlEvents`): the `id` of every
 * `unit` and the text of its first `target`, nested inline elements included.
 */
function parseXliff(text: string): ParseResult {
  const events = xmlEvents(text);
  if (events === undefined || !wellNested(events, "xliff")) {
    return malformed;
  }
  const units = new XliffUnits();
  return events.every((e) => units.take(e)) ? { ok: true, entries: units.entries } : malformed;
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
