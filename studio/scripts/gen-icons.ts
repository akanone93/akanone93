// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The icon fonts of the plugin canvas (THM-005, PC-03), derived from the fonts the server subsets for each release
 * (`backend/internal/icons/fonts`, pinned by icons.lock): Material Symbols Outlined at its default instance (fill 0,
 * weight 400, grade 0, optical size 24 — the defaults of Flutter's `Icon`) and the Cupertino icons, each reduced to its
 * named icons with a format 12 `cmap`, without variations, layout tables or glyph names. Material Symbols is split into
 * parts under the repository's 500 KB file limit; the canvas lists them as fallbacks of one another. The name lists
 * and the fonts' licences are copied as they are. The output depends on the inputs only; `gen-icons.test.ts` regenerates
 * it and compares.
 *
 *   bun scripts/gen-icons.ts        writes studio/apps/web/src/features/plugin-canvas/icons/
 */

const KEPT = ["OS/2", "cvt ", "fpgm", "gasp", "head", "hhea", "hmtx", "maxp", "prep"];
/** The name IDs a font's family is known by, replaced by the part's own family; the rest (copyright, licence) stay. */
const FAMILY_NAMES = new Set([1, 2, 3, 4, 6, 16, 17, 21, 22, 25]);
/** The most glyph bytes one part of Material Symbols holds. */
const PART_BYTES = 360_000;

interface Font {
  readonly bytes: Uint8Array;
  readonly view: DataView;
  readonly tables: ReadonlyMap<string, { offset: number; length: number }>;
}

function parse(bytes: Uint8Array): Font {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const tables = new Map<string, { offset: number; length: number }>();
  for (let i = 0; i < view.getUint16(4); i++) {
    const at = 12 + 16 * i;
    tables.set(String.fromCharCode(...bytes.subarray(at, at + 4)), {
      offset: view.getUint32(at + 8),
      length: view.getUint32(at + 12),
    });
  }
  return { bytes, view, tables };
}

function table(font: Font, tag: string): Uint8Array {
  const t = font.tables.get(tag);
  if (t === undefined) {
    throw new Error(`the font has no ${tag} table`);
  }
  return font.bytes.slice(t.offset, t.offset + t.length);
}

/** The glyph of each code point, from the font's format 12 (or format 4) Unicode subtable. */
function glyphsOf(font: Font): Map<number, number> {
  const cmap = font.tables.get("cmap");
  if (cmap === undefined) {
    throw new Error("the font has no cmap table");
  }
  const v = font.view;
  const out = new Map<number, number>();
  for (let i = 0; i < v.getUint16(cmap.offset + 2); i++) {
    const sub = cmap.offset + v.getUint32(cmap.offset + 4 + 8 * i + 4);
    const format = v.getUint16(sub);
    if (format === 12) {
      for (let g = 0; g < v.getUint32(sub + 12); g++) {
        const at = sub + 16 + 12 * g;
        const [start, end, glyph] = [v.getUint32(at), v.getUint32(at + 4), v.getUint32(at + 8)];
        for (let c = start; c <= end; c++) {
          out.set(c, glyph + c - start);
        }
      }
    } else if (format === 4) {
      format4(v, sub, out);
    }
  }
  return out;
}

function format4(v: DataView, sub: number, out: Map<number, number>): void {
  const segments = v.getUint16(sub + 6) / 2;
  const ends = sub + 14;
  const starts = ends + 2 * segments + 2;
  const deltas = starts + 2 * segments;
  const ranges = deltas + 2 * segments;
  for (let s = 0; s < segments; s++) {
    const [end, start, delta, range] = [
      v.getUint16(ends + 2 * s),
      v.getUint16(starts + 2 * s),
      v.getInt16(deltas + 2 * s),
      v.getUint16(ranges + 2 * s),
    ];
    for (let c = start; c <= end && c !== 0xffff; c++) {
      const g = range === 0 ? c + delta : v.getUint16(ranges + 2 * s + range + 2 * (c - start));
      if (!out.has(c) && g !== 0) {
        out.set(c, (range === 0 ? g : g + delta) & 0xffff);
      }
    }
  }
}

/** Each glyph's outline bytes, by glyph ID (a long `loca`). */
function outlinesOf(font: Font): Uint8Array[] {
  const glyf = font.tables.get("glyf");
  const loca = font.tables.get("loca");
  if (glyf === undefined || loca === undefined) {
    throw new Error("the font has no TrueType outlines");
  }
  const long = font.view.getInt16((font.tables.get("head")?.offset ?? 0) + 50) === 1;
  const at = (i: number) =>
    long ? font.view.getUint32(loca.offset + 4 * i) : 2 * font.view.getUint16(loca.offset + 2 * i);
  const count = font.view.getUint16((font.tables.get("maxp")?.offset ?? 0) + 4);
  return Array.from({ length: count }, (_, i) => font.bytes.slice(glyf.offset + at(i), glyf.offset + at(i + 1)));
}

/**
 * `name` with the family of every name record replaced by `family` (Windows, US English): the canvas registers a font
 * under its own family name, so each part needs one of its own. Copyright, licence and the other records are kept.
 */
function namesOf(font: Font, family: string): Uint8Array {
  const name = table(font, "name");
  const v = new DataView(name.buffer);
  const storage = v.getUint16(4);
  const records: { platform: number; encoding: number; language: number; id: number; text: Uint8Array }[] = [];
  for (let i = 0; i < v.getUint16(2); i++) {
    const at = 6 + 12 * i;
    const id = v.getUint16(at + 6);
    if (!FAMILY_NAMES.has(id)) {
      const length = v.getUint16(at + 8);
      const offset = storage + v.getUint16(at + 10);
      records.push({
        platform: v.getUint16(at),
        encoding: v.getUint16(at + 2),
        language: v.getUint16(at + 4),
        id,
        text: name.slice(offset, offset + length),
      });
    }
  }
  const utf16 = (text: string) => {
    const out = new Uint8Array(2 * text.length);
    const v16 = new DataView(out.buffer);
    [...text].forEach((ch, i) => {
      v16.setUint16(2 * i, ch.charCodeAt(0));
    });
    return out;
  };
  const own: [number, string][] = [
    [1, family],
    [2, "Regular"],
    [4, family],
    [6, family.replaceAll(" ", "")],
  ];
  for (const [id, text] of own) {
    records.push({ platform: 3, encoding: 1, language: 0x409, id, text: utf16(text) });
  }
  records.sort((a, b) => a.platform - b.platform || a.encoding - b.encoding || a.language - b.language || a.id - b.id);
  const header = 6 + 12 * records.length;
  const out = new Uint8Array(header + records.reduce((n, r) => n + r.text.length, 0));
  const o = new DataView(out.buffer);
  o.setUint16(2, records.length);
  o.setUint16(4, header);
  let offset = 0;
  records.forEach((r, i) => {
    const at = 6 + 12 * i;
    [r.platform, r.encoding, r.language, r.id, r.text.length, offset].forEach((n, k) => {
      o.setUint16(at + 2 * k, n);
    });
    out.set(r.text, header + offset);
    offset += r.text.length;
  });
  return out;
}

/** A format 12 subtable mapping `glyphs` (code point to glyph ID), under Unicode full and Windows full. */
function cmapOf(glyphs: ReadonlyMap<number, number>): Uint8Array {
  const sorted = [...glyphs].sort(([a], [b]) => a - b);
  const groups: [number, number, number][] = [];
  for (const [c, g] of sorted) {
    const last = groups.at(-1);
    if (last !== undefined && c === last[1] + 1 && g === last[2] + c - last[0]) {
      last[1] = c;
    } else {
      groups.push([c, c, g]);
    }
  }
  const subLength = 16 + 12 * groups.length;
  const out = new Uint8Array(4 + 8 * 2 + subLength);
  const v = new DataView(out.buffer);
  v.setUint16(2, 2);
  [
    [0, 4],
    [3, 10],
  ].forEach(([platform, encoding], i) => {
    v.setUint16(4 + 8 * i, platform as number);
    v.setUint16(6 + 8 * i, encoding as number);
    v.setUint32(8 + 8 * i, 20);
  });
  v.setUint16(20, 12);
  v.setUint32(24, subLength);
  v.setUint32(32, groups.length);
  groups.forEach(([start, end, glyph], i) => {
    v.setUint32(36 + 12 * i, start);
    v.setUint32(40 + 12 * i, end);
    v.setUint32(44 + 12 * i, glyph);
  });
  return out;
}

function checksum(bytes: Uint8Array): number {
  const padded = new Uint8Array(Math.ceil(bytes.length / 4) * 4);
  padded.set(bytes);
  const v = new DataView(padded.buffer);
  let sum = 0;
  for (let i = 0; i < padded.length; i += 4) {
    sum = (sum + v.getUint32(i)) >>> 0;
  }
  return sum;
}

/** A TrueType font of `tables`, sorted by tag, with its checksums. */
function assemble(tables: ReadonlyMap<string, Uint8Array>): Uint8Array {
  const sorted = [...tables].sort(([a], [b]) => (a < b ? -1 : 1));
  const n = sorted.length;
  const log = Math.floor(Math.log2(n));
  let size = 12 + 16 * n;
  for (const [, data] of sorted) {
    size += Math.ceil(data.length / 4) * 4;
  }
  const out = new Uint8Array(size);
  const v = new DataView(out.buffer);
  v.setUint32(0, 0x00010000);
  v.setUint16(4, n);
  v.setUint16(6, 16 * 2 ** log);
  v.setUint16(8, log);
  v.setUint16(10, 16 * n - 16 * 2 ** log);
  let offset = 12 + 16 * n;
  let head = 0;
  sorted.forEach(([tag, data], i) => {
    const at = 12 + 16 * i;
    out.set(
      [...tag].map((ch) => ch.charCodeAt(0)),
      at,
    );
    v.setUint32(at + 4, checksum(data));
    v.setUint32(at + 8, offset);
    v.setUint32(at + 12, data.length);
    out.set(data, offset);
    head = tag === "head" ? offset : head;
    offset += Math.ceil(data.length / 4) * 4;
  });
  v.setUint32(head + 8, (0xb1b0afba - checksum(out)) >>> 0);
  return out;
}

/**
 * `font` with only the glyphs of `codePoints` (glyph IDs kept, every other outline emptied), at its default instance:
 * the variation tables, layout tables and glyph names are dropped.
 */
function reduced(
  font: Font,
  outlines: readonly Uint8Array[],
  glyphs: ReadonlyMap<number, number>,
  family: string,
): Uint8Array {
  const keep = new Set([0, ...glyphs.values()]);
  const loca = new Uint8Array(4 * (outlines.length + 1));
  const lv = new DataView(loca.buffer);
  const parts: Uint8Array[] = [];
  let at = 0;
  outlines.forEach((outline, i) => {
    lv.setUint32(4 * i, at);
    if (keep.has(i)) {
      const padded = new Uint8Array(Math.ceil(outline.length / 4) * 4);
      padded.set(outline);
      parts.push(padded);
      at += padded.length;
    }
  });
  lv.setUint32(4 * outlines.length, at);
  const glyf = new Uint8Array(at);
  let o = 0;
  for (const p of parts) {
    glyf.set(p, o);
    o += p.length;
  }
  const tables = new Map<string, Uint8Array>();
  for (const tag of KEPT) {
    if (font.tables.has(tag)) {
      tables.set(tag, table(font, tag));
    }
  }
  const head = tables.get("head") as Uint8Array;
  const hv = new DataView(head.buffer);
  hv.setUint32(8, 0);
  hv.setInt16(50, 1);
  const post = table(font, "post").slice(0, 32);
  new DataView(post.buffer).setUint32(0, 0x00030000);
  tables.set("post", post);
  tables.set("cmap", cmapOf(glyphs));
  tables.set("name", namesOf(font, family));
  tables.set("glyf", glyf);
  tables.set("loca", loca);
  return assemble(tables);
}

/** The code points of a `<name> <hex code point>` list. */
function codePointsOf(names: string): number[] {
  const points = names
    .split("\n")
    .map((line) => line.trim().split(" ")[1])
    .filter((hex): hex is string => hex !== undefined && hex !== "")
    .map((hex) => Number.parseInt(hex, 16));
  return [...new Set(points)].sort((a, b) => a - b);
}

/**
 * The named icons of `bytes`, in parts of at most `limit` glyph bytes (one part when `limit` is infinite), each in the
 * family `family` and its number.
 */
function iconParts(bytes: Uint8Array, names: string, limit: number, family: string): Uint8Array[] {
  const font = parse(bytes);
  const all = glyphsOf(font);
  const outlines = outlinesOf(font);
  const parts: Map<number, number>[] = [new Map()];
  let size = 0;
  for (const c of codePointsOf(names)) {
    const g = all.get(c);
    if (g === undefined) {
      continue;
    }
    const length = outlines[g]?.length ?? 0;
    if (size + length > limit) {
      parts.push(new Map());
      size = 0;
    }
    parts.at(-1)?.set(c, g);
    size += length;
  }
  return parts.map((glyphs, i) => reduced(font, outlines, glyphs, parts.length === 1 ? family : `${family} ${i + 1}`));
}

/** The files of the canvas's icons, by name, from the server's fonts and name lists. */
export function iconFiles(sources: {
  readonly material: Uint8Array;
  readonly cupertino: Uint8Array;
  readonly materialNames: string;
  readonly cupertinoNames: string;
  readonly mirrored: string;
  readonly licences: { readonly material: string; readonly cupertino: string };
}): Map<string, Uint8Array | string> {
  const files = new Map<string, Uint8Array | string>();
  iconParts(sources.material, sources.materialNames, PART_BYTES, "Plux Material Symbols").forEach((font, i) => {
    files.set(`MaterialSymbols-${i + 1}.ttf`, font);
  });
  files.set(
    "CupertinoIcons.ttf",
    iconParts(
      sources.cupertino,
      sources.cupertinoNames,
      Number.POSITIVE_INFINITY,
      "Plux Cupertino Icons",
    )[0] as Uint8Array,
  );
  files.set("material.codepoints", sources.materialNames);
  files.set("cupertino.codepoints", sources.cupertinoNames);
  files.set("mirrored.txt", sources.mirrored);
  files.set("LICENSE-material-design-icons", sources.licences.material);
  files.set("LICENSE-cupertino-icons", sources.licences.cupertino);
  return files;
}

/** The server's icon sources, read from the repository. */
export async function iconSources(root: URL): Promise<Parameters<typeof iconFiles>[0]> {
  const at = (name: string) => Bun.file(new URL(`backend/internal/icons/fonts/${name}`, root));
  return {
    material: new Uint8Array(await at("MaterialSymbolsOutlined.ttf").arrayBuffer()),
    cupertino: new Uint8Array(await at("CupertinoIcons.ttf").arrayBuffer()),
    materialNames: await at("material.codepoints").text(),
    cupertinoNames: await at("cupertino.codepoints").text(),
    mirrored: await at("mirrored.txt").text(),
    licences: {
      material: await at("LICENSE-material-design-icons").text(),
      cupertino: await at("LICENSE-cupertino-icons").text(),
    },
  };
}

if (import.meta.main) {
  const root = new URL("../../", import.meta.url);
  const out = new URL("studio/apps/web/src/features/plugin-canvas/icons/", root);
  for (const [name, data] of iconFiles(await iconSources(root))) {
    await Bun.write(new URL(name, out), data);
  }
}
