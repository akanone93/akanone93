// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

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

/** Derives a pseudo-locale catalogue from the source catalogue; used at load time, never stored. */
export function pseudoCatalogue(
  locale: "en-XA" | "ar-XB",
  source: Readonly<Record<string, string>>,
): Record<string, string> {
  const fn = locale === "en-XA" ? pseudoAccent : pseudoMirror;
  return Object.fromEntries(Object.entries(source).map(([id, message]) => [id, fn(message)]));
}
