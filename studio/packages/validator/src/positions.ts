// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** A 1-based line and column in UTF-16 units, as Monaco counts them. */
export interface LineColumn {
  readonly lineNumber: number;
  readonly column: number;
}

/**
 * The language service counts code points (CMP-004); editors count UTF-16 units. These two
 * functions convert at the boundary so an emoji or an astral letter never shifts a range.
 */
export function codePointsBefore(text: string, index: number): number {
  const end = Math.max(0, Math.min(index, text.length));
  let count = 0;
  for (let i = 0; i < end; i++) {
    const c = text.charCodeAt(i);
    // The low half of a surrogate pair belongs to the code point its high half started.
    if (!(c >= 0xdc00 && c <= 0xdfff && i > 0 && isHigh(text.charCodeAt(i - 1)))) {
      count++;
    }
  }
  return count;
}

function isHigh(c: number): boolean {
  return c >= 0xd800 && c <= 0xdbff;
}

/** The UTF-16 index after `codePoints` code points; the length of the text when it has fewer. */
export function utf16Index(text: string, codePoints: number): number {
  let index = 0;
  for (let n = 0; n < codePoints && index < text.length; n++) {
    index += isHigh(text.charCodeAt(index)) && index + 1 < text.length ? 2 : 1;
  }
  return index;
}

/** The line and column of a UTF-16 offset; lines end at `\n`. */
export function positionAt(text: string, offset: number): LineColumn {
  const end = Math.max(0, Math.min(offset, text.length));
  let lineNumber = 1;
  let lineStart = 0;
  for (let i = 0; i < end; i++) {
    if (text.charCodeAt(i) === 10) {
      lineNumber++;
      lineStart = i + 1;
    }
  }
  return { lineNumber, column: end - lineStart + 1 };
}

/** The UTF-16 offset of a line and column; clamped to the text. */
export function offsetAt(text: string, position: LineColumn): number {
  let start = 0;
  for (let line = 1; line < position.lineNumber; line++) {
    const next = text.indexOf("\n", start);
    if (next < 0) {
      return text.length;
    }
    start = next + 1;
  }
  const lineEnd = text.indexOf("\n", start);
  const limit = lineEnd < 0 ? text.length : lineEnd;
  return Math.min(start + Math.max(0, position.column - 1), limit);
}
