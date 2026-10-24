// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Messages } from "@lingui/core";

const accents: Readonly<Record<string, string>> = {
  a: "á",
  b: "ƀ",
  c: "ç",
  d: "ð",
  e: "é",
  f: "ƒ",
  g: "ĝ",
  h: "ĥ",
  i: "î",
  j: "ĵ",
  k: "ķ",
  l: "ļ",
  m: "ɱ",
  n: "ñ",
  o: "ö",
  p: "þ",
  q: "ǫ",
  r: "ŕ",
  s: "š",
  t: "ţ",
  u: "û",
  v: "ṽ",
  w: "ŵ",
  x: "ẋ",
  y: "ý",
  z: "ž",
  A: "Á",
  B: "Ɓ",
  C: "Ç",
  D: "Ð",
  E: "É",
  F: "Ƒ",
  G: "Ĝ",
  H: "Ĥ",
  I: "Î",
  J: "Ĵ",
  K: "Ķ",
  L: "Ļ",
  M: "Ṁ",
  N: "Ñ",
  O: "Ö",
  P: "Þ",
  Q: "Ǫ",
  R: "Ŕ",
  S: "Š",
  T: "Ţ",
  U: "Û",
  V: "Ṽ",
  W: "Ŵ",
  X: "Ẋ",
  Y: "Ý",
  Z: "Ž",
};

const RLM = "‏";
const RLE = "‫";
const PDF = "‬";

/** Applies `fn` to the literal text of an ICU message, leaving `{placeholders}` and `<tags>` untouched. */
function mapLiteral(message: string, fn: (text: string) => string): string {
  let out = "";
  let literal = "";
  let depth = 0;
  const flush = () => {
    out += fn(literal);
    literal = "";
  };
  for (const ch of message) {
    if (ch === "{") {
      if (depth === 0) {
        flush();
      }
      depth++;
      out += ch;
    } else if (ch === "}" && depth > 0) {
      depth--;
      out += ch;
    } else if (depth > 0) {
      out += ch;
    } else {
      literal += ch;
    }
  }
  flush();
  return out;
}

/** en-XA: accented letters and 40 % longer (ceil), to expose clipping and concatenation (I18N-010). */
export function pseudoAccent(message: string): string {
  let letters = 0;
  const accented = mapLiteral(message, (text) =>
    [...text]
      .map((ch) => {
        if (/[A-Za-z]/.test(ch)) {
          letters++;
        }
        return accents[ch] ?? ch;
      })
      .join(""),
  );
  const pad = Math.ceil(letters * 0.4);
  return `[${accented}${"~".repeat(pad)}]`;
}

/** ar-XB: the English text forced right-to-left with explicit bidi marks, to expose mirroring bugs. */
export function pseudoMirror(message: string): string {
  return `${RLE}${mapLiteral(message, (text) => (text === "" ? text : `${RLM}${text}${RLM}`))}${PDF}`;
}

/** A compiled Lingui message: plain text, or literal parts mixed with placeholder tokens. */
type Compiled = Messages[string];
type Token = Exclude<Compiled, string>[number];

/**
 * Pseudo-localises one compiled message: every literal part, including the branches of plural
 * and select forms, is accented (en-XA) or wrapped in right-to-left marks (ar-XB); placeholders
 * stay as they are (rule book §7.7, I18N-010).
 */
export function pseudoCompiled(locale: "en-XA" | "ar-XB", message: Compiled): Compiled {
  let letters = 0;
  const literal = (text: string): string => {
    if (locale === "ar-XB") {
      return text === "" ? text : `${RLM}${text}${RLM}`;
    }
    return [...text]
      .map((ch) => {
        if (/[A-Za-z]/.test(ch)) {
          letters++;
        }
        return accents[ch] ?? ch;
      })
      .join("");
  };
  const walk = (m: Compiled): Compiled => (typeof m === "string" ? literal(m) : m.map(token));
  const token = (t: Token): Token => {
    if (typeof t === "string") {
      return literal(t);
    }
    const [name, format, options] = t;
    if (options === undefined || typeof options !== "object" || options === null) {
      return t;
    }
    const mapped = Object.fromEntries(Object.entries(options).map(([k, v]) => [k, walk(v as Compiled)]));
    return [name, format, mapped] as Token;
  };
  const body = walk(message);
  const [open, close] = locale === "ar-XB" ? [RLE, PDF] : ["[", `${"~".repeat(Math.ceil(letters * 0.4))}]`];
  return typeof body === "string" ? `${open}${body}${close}` : [open, ...body, close];
}

/** Derives a pseudo-locale catalogue from the compiled source catalogue; used at load time, never stored. */
export function pseudoCatalogue(locale: "en-XA" | "ar-XB", source: Readonly<Messages>): Messages {
  return Object.fromEntries(Object.entries(source).map(([id, message]) => [id, pseudoCompiled(locale, message)]));
}
