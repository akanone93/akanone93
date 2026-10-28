// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { fontFamilyName } from "./font-family.ts";

interface NameRecord {
  readonly platform: number;
  readonly language?: number;
  readonly nameId: number;
  readonly text: string;
}

function utf16be(text: string): number[] {
  return [...text].flatMap((c) => [(c.charCodeAt(0) >> 8) & 0xff, c.charCodeAt(0) & 0xff]);
}

/** A minimal sfnt whose only table is `name` with `records`. */
function font(records: readonly NameRecord[], version = 0x00010000): Uint8Array {
  const strings = records.map((r) => (r.platform === 3 ? utf16be(r.text) : [...r.text].map((c) => c.charCodeAt(0))));
  const storage = 6 + records.length * 12;
  const name = new Uint8Array(storage + strings.reduce((n, s) => n + s.length, 0));
  const nv = new DataView(name.buffer);
  nv.setUint16(2, records.length);
  nv.setUint16(4, storage);
  let at = 0;
  records.forEach((r, i) => {
    const o = 6 + i * 12;
    nv.setUint16(o, r.platform);
    nv.setUint16(o + 2, r.platform === 3 ? 1 : 0);
    nv.setUint16(o + 4, r.language ?? (r.platform === 3 ? 0x409 : 0));
    nv.setUint16(o + 6, r.nameId);
    nv.setUint16(o + 8, strings[i]?.length ?? 0);
    nv.setUint16(o + 10, at);
    name.set(strings[i] ?? [], storage + at);
    at += strings[i]?.length ?? 0;
  });
  const out = new Uint8Array(12 + 16 + name.length);
  const ov = new DataView(out.buffer);
  ov.setUint32(0, version);
  ov.setUint16(4, 1);
  ov.setUint32(12, 0x6e616d65);
  ov.setUint32(20, 28);
  ov.setUint32(24, name.length);
  out.set(name, 28);
  return out;
}

describe("STU-006 font family names", () => {
  test("a TrueType font gives its Windows family name", () => {
    expect(fontFamilyName(font([{ platform: 3, nameId: 1, text: "Brand Sans" }]))).toBe("Brand Sans");
  });

  test("an OpenType (CFF) font is read the same way", () => {
    expect(fontFamilyName(font([{ platform: 3, nameId: 1, text: "Brand Serif" }], 0x4f54544f))).toBe("Brand Serif");
  });

  test("the typographic family wins over the family", () => {
    const records: NameRecord[] = [
      { platform: 3, nameId: 1, text: "Brand Sans Light" },
      { platform: 3, nameId: 16, text: "Brand Sans" },
    ];
    expect(fontFamilyName(font(records))).toBe("Brand Sans");
  });

  test("a Windows string wins over a Macintosh one, a Macintosh one is used alone", () => {
    expect(
      fontFamilyName(
        font([
          { platform: 1, nameId: 1, text: "Mac Name" },
          { platform: 3, nameId: 1, text: "Win Name" },
        ]),
      ),
    ).toBe("Win Name");
    expect(fontFamilyName(font([{ platform: 1, nameId: 1, text: "Mac Name" }]))).toBe("Mac Name");
  });

  test("English wins over another language; non-ASCII names decode", () => {
    const records: NameRecord[] = [
      { platform: 3, language: 0x407, nameId: 1, text: "Schrift" },
      { platform: 3, nameId: 1, text: "Größe" },
    ];
    expect(fontFamilyName(font(records))).toBe("Größe");
  });

  test("a font with no family record gives null", () => {
    expect(fontFamilyName(font([{ platform: 3, nameId: 4, text: "Brand Sans Bold" }]))).toBeNull();
    expect(fontFamilyName(font([{ platform: 3, nameId: 1, text: "   " }]))).toBeNull();
  });

  test("truncated input gives null at every length", () => {
    const whole = font([{ platform: 3, nameId: 1, text: "Brand Sans" }]);
    for (let n = 0; n < whole.length; n++) {
      expect(fontFamilyName(whole.subarray(0, n))).toBeNull();
    }
  });

  test("bogus offsets and counts give null", () => {
    const base = () => font([{ platform: 3, nameId: 1, text: "Brand Sans" }]);
    const table = base();
    new DataView(table.buffer).setUint32(20, 0xffffff00);
    expect(fontFamilyName(table)).toBeNull();
    const length = base();
    new DataView(length.buffer).setUint32(24, 0xffffffff);
    expect(fontFamilyName(length)).toBeNull();
    const tables = base();
    new DataView(tables.buffer).setUint16(4, 0xffff);
    expect(fontFamilyName(tables)).toBeNull();
    const count = base();
    new DataView(count.buffer).setUint16(30, 0xffff);
    expect(fontFamilyName(count)).toBeNull();
    const storage = base();
    new DataView(storage.buffer).setUint16(32, 0xffff);
    expect(fontFamilyName(storage)).toBeNull();
    const string = base();
    new DataView(string.buffer).setUint16(28 + 6 + 10, 0xffff);
    expect(fontFamilyName(string)).toBeNull();
  });

  test("something that is not a single font gives null", () => {
    expect(fontFamilyName(new Uint8Array(0))).toBeNull();
    expect(fontFamilyName(new TextEncoder().encode("<html>not a font</html>"))).toBeNull();
    expect(fontFamilyName(font([{ platform: 3, nameId: 1, text: "X" }], 0x74746366))).toBeNull();
  });
});
