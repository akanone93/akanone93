// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * ICU MessageFormat for the canvas (PC-32, I18N-001): a translation's message formatted with its arguments in the
 * previewed locale — `{name}`; `number` (with `percent` or `integer`), `date` and `time` (with `short` to `full`; a
 * skeleton shows the medium style); `plural` with `offset:`, exact `=n` matches and `#`; `selectordinal`; `select`;
 * nesting and `'` quoting — so a plural reads "3 items", not its pattern. Digits are Latin, the device's default
 * (I18N-003). An argument the message names but the canvas
 * has no value for keeps its `{name}`.
 */

type Args = Readonly<Record<string, unknown>>;

interface Cursor {
  readonly text: string;
  at: number;
}

/**
 * At a `'`: the text it quotes — `''` is one apostrophe, `'{…}'` keeps the braces as text — with the cursor after it;
 * undefined when the apostrophe quotes nothing and is itself text.
 */
function quoted(c: Cursor): string | undefined {
  const next = c.text[c.at + 1];
  if (next === "'") {
    c.at += 2;
    return "'";
  }
  if (next !== "{" && next !== "}" && next !== "#") {
    return undefined;
  }
  const end = c.text.indexOf("'", c.at + 1);
  const stop = end < 0 ? c.text.length : end;
  const out = c.text.slice(c.at + 1, stop);
  c.at = end < 0 ? c.text.length : end + 1;
  return out;
}

/** The text up to the next `{` or `}` at this level (and `#` inside a plural), with `'` quoting undone. */
function literal(c: Cursor, hash: string | undefined): string {
  let out = "";
  while (c.at < c.text.length) {
    const ch = c.text[c.at] as string;
    const q = ch === "'" ? quoted(c) : undefined;
    if (q !== undefined) {
      out += q;
      continue;
    }
    if (ch === "{" || ch === "}") {
      return out;
    }
    out += ch === "#" && hash !== undefined ? hash : ch;
    c.at += 1;
  }
  return out;
}

const skipSpace = (c: Cursor) => {
  while (/\s/.test(c.text[c.at] ?? "")) {
    c.at += 1;
  }
};

/** A word: an argument name, a type or a selector. */
function word(c: Cursor): string {
  skipSpace(c);
  const start = c.at;
  while (c.at < c.text.length && !/[\s,{}]/.test(c.text[c.at] as string)) {
    c.at += 1;
  }
  return c.text.slice(start, c.at);
}

/** The text of a `{…}` block, skipping nested blocks, with the cursor after its `}`. */
function block(c: Cursor): string {
  skipSpace(c);
  if (c.text[c.at] !== "{") {
    return "";
  }
  let depth = 0;
  const start = c.at + 1;
  for (; c.at < c.text.length; c.at += 1) {
    const ch = c.text[c.at];
    if (ch === "{") {
      depth += 1;
    } else if (ch === "}") {
      depth -= 1;
      if (depth === 0) {
        c.at += 1;
        return c.text.slice(start, c.at - 1);
      }
    }
  }
  return c.text.slice(start);
}

/** The branches of a `plural`, `selectordinal` or `select`, by selector, and its `offset:`. */
function branches(c: Cursor): { readonly options: Map<string, string>; readonly offset: number } {
  const options = new Map<string, string>();
  let offset = 0;
  for (;;) {
    skipSpace(c);
    if (c.at >= c.text.length || c.text[c.at] === "}") {
      c.at += 1;
      return { options, offset };
    }
    const key = word(c);
    if (key.startsWith("offset:")) {
      offset = Number(key.slice(7)) || 0;
      continue;
    }
    options.set(key, block(c));
  }
}

const DATE_STYLES: ReadonlySet<string> = new Set(["short", "medium", "long", "full"]);

/** A `number`, `date` or `time` argument's value in `locale` with its style; undefined when it cannot be shown. */
function styled(type: string, style: string, v: unknown, locale: string): string | undefined {
  if (type === "number") {
    if (typeof v !== "number") {
      return undefined;
    }
    const options: Intl.NumberFormatOptions =
      style === "percent" ? { style: "percent" } : style === "integer" ? { maximumFractionDigits: 0 } : {};
    return new Intl.NumberFormat(locale, { ...options, numberingSystem: "latn" }).format(v);
  }
  const at = typeof v === "number" || typeof v === "string" ? new Date(v) : undefined;
  if (at === undefined || Number.isNaN(at.getTime())) {
    return undefined;
  }
  // A design-time value is shown in UTC, as the mock data writes it, so the canvas is the same on every machine.
  const kind = DATE_STYLES.has(style) ? (style as "short" | "medium" | "long" | "full") : "medium";
  const options: Intl.DateTimeFormatOptions = type === "date" ? { dateStyle: kind } : { timeStyle: kind };
  return new Intl.DateTimeFormat(locale, { ...options, timeZone: "UTC", numberingSystem: "latn" }).format(at);
}

/** A `{name, number|date|time[, style]}` argument after its type, with the cursor after its `}`. */
function styledArgument(c: Cursor, type: string, name: string, args: Args, locale: string): string {
  let style = "";
  if (c.text[c.at] === ",") {
    c.at += 1;
    skipSpace(c);
    const end = c.text.indexOf("}", c.at);
    style = c.text.slice(c.at, end < 0 ? c.text.length : end).trim();
    c.at = end < 0 ? c.text.length : end;
  }
  skipSpace(c);
  c.at += 1;
  return styled(type, style, args[name], locale) ?? `{${name}}`;
}

/** A `plural`, `selectordinal` or `select` argument after its type, with the cursor after its `}`. */
function choice(c: Cursor, type: string, name: string, args: Args, locale: string, hash: string | undefined): string {
  if (c.text[c.at] === ",") {
    c.at += 1;
  }
  const { options, offset } = branches(c);
  const v = args[name];
  if (type === "select") {
    return format(options.get(String(v)) ?? options.get("other") ?? "", args, locale, hash);
  }
  if (typeof v !== "number") {
    return format(options.get("other") ?? "", args, locale, `{${name}}`);
  }
  const shown = new Intl.NumberFormat(locale, { numberingSystem: "latn" }).format(v - offset);
  const rules = new Intl.PluralRules(locale, { type: type === "selectordinal" ? "ordinal" : "cardinal" });
  const picked = options.get(`=${v}`) ?? options.get(rules.select(v - offset)) ?? options.get("other") ?? "";
  return format(picked, args, locale, shown);
}

const STYLED: ReadonlySet<string> = new Set(["number", "date", "time"]);

function argument(c: Cursor, args: Args, locale: string, hash: string | undefined): string {
  const name = word(c);
  skipSpace(c);
  if (c.text[c.at] === "}") {
    c.at += 1;
    const v = args[name];
    return v === undefined ? `{${name}}` : String(v);
  }
  c.at += 1;
  const type = word(c);
  skipSpace(c);
  return STYLED.has(type) ? styledArgument(c, type, name, args, locale) : choice(c, type, name, args, locale, hash);
}

/** `message` with its arguments filled in `locale`. */
export function format(message: string, args: Args, locale: string, hash?: string): string {
  const c: Cursor = { text: message, at: 0 };
  let out = "";
  while (c.at < c.text.length) {
    out += literal(c, hash);
    if (c.text[c.at] === "{") {
      c.at += 1;
      out += argument(c, args, locale, hash);
    } else if (c.text[c.at] === "}") {
      c.at += 1;
    }
  }
  return out;
}

/** The names of the plural and selectordinal arguments of `message`, for the canvas's samples. */
export function pluralArgs(message: string): string[] {
  return [...message.matchAll(/\{\s*(\w+)\s*,\s*(?:plural|selectordinal)\s*,/g)].map((m) => m[1] as string);
}
