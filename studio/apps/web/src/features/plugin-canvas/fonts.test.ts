// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { CANVAS_ARROW } from "@plux/canvas";

const dir = new URL("./fonts/", import.meta.url);
const pinned = readFileSync(new URL("VERSION", dir), "utf-8")
  .split("\n")
  .filter((line) => line.startsWith("file "))
  .map((line) => line.split(" ").slice(1) as [string, string]);

/** Adds the code points of a cmap format 4 subtable at `at` that map to a glyph. */
function format4(v: DataView, at: number, out: Set<number>): void {
  const width = v.getUint16(at + 6);
  const ends = at + 14;
  const starts = ends + width + 2;
  const deltas = starts + width;
  const ranges = deltas + width;
  for (let s = 0; s < width; s += 2) {
    const start = v.getUint16(starts + s);
    const delta = v.getInt16(deltas + s);
    const range = v.getUint16(ranges + s);
    const glyph = (c: number) => {
      const raw = range === 0 ? c : v.getUint16(ranges + s + range + (c - start) * 2);
      return raw === 0 ? 0 : (raw + delta) & 0xffff;
    };
    for (let c = start; c <= v.getUint16(ends + s) && c !== 0xffff; c++) {
      if (glyph(c) !== 0) {
        out.add(c);
      }
    }
  }
}

/** Adds the code points of a cmap format 12 subtable at `at`. */
function format12(v: DataView, at: number, out: Set<number>): void {
  for (let g = 0; g < v.getUint32(at + 12); g++) {
    const group = at + 16 + g * 12;
    for (let c = v.getUint32(group); c <= v.getUint32(group + 4); c++) {
      out.add(c);
    }
  }
}

/** The code points a TrueType font maps to a glyph (cmap formats 4 and 12). */
function codePoints(bytes: Buffer): Set<number> {
  const v = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const records = Array.from({ length: v.getUint16(4) }, (_, i) => 12 + i * 16);
  const record = records.find((r) => bytes.toString("latin1", r, r + 4) === "cmap") ?? 0;
  const cmap = v.getUint32(record + 8);
  const out = new Set<number>();
  for (let i = 0; i < v.getUint16(cmap + 2); i++) {
    const at = cmap + v.getUint32(cmap + 8 + i * 8);
    const read = { 4: format4, 12: format12 }[v.getUint16(at)];
    read?.(v, at, out);
  }
  return out;
}

describe("[STU-006] canvas fonts", () => {
  test("four fonts are pinned", () => {
    expect(pinned.map(([name]) => name).sort()).toEqual([
      "NotoSans_400Regular.ttf",
      "NotoSans_700Bold.ttf",
      "Roboto_400Regular.ttf",
      "Roboto_700Bold.ttf",
    ]);
  });

  for (const [name, sha] of pinned) {
    test(`${name} matches its pinned SHA-256`, () => {
      expect(
        createHash("sha256")
          .update(readFileSync(new URL(name, dir)))
          .digest("hex"),
      ).toBe(sha);
    });
  }

  // The canvas draws arrow titles and badges with these fonts: a character none of them has draws as a box.
  test("every font draws the characters of arrow titles and badges", () => {
    for (const [name] of pinned) {
      const has = codePoints(readFileSync(new URL(name, dir)));
      const missing = [...`${CANVAS_ARROW}·0123456789()`].filter((ch) => !has.has(ch.codePointAt(0) ?? 0));
      expect({ name, missing }).toEqual({ name, missing: [] });
    }
  });
});
