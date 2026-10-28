// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Writes a `.plux` archive (ADR-0102, STU-024): a ZIP of the SCH-006 project layout and a manifest, in the form the
 * server writes. Entries are sorted by name, stored without compression, carry the ZIP epoch (1980-01-01) as time
 * and mode 0644, and have no extra fields and no comment. The server reads the result as untrusted input and
 * refuses it on any disagreement, so the format is followed exactly.
 */

export interface ArchiveFile {
  /** A path inside the project, such as `app.json` or `plugins/main/plugin.json`. */
  readonly path: string;
  readonly content: Uint8Array;
}

export interface ArchiveApp {
  readonly key: string;
  readonly name: string;
  readonly description: string;
  readonly tags: readonly string[];
}

const manifestName = "plux-archive.json";
const projectPrefix = "project/";
/** 1980-01-01 in the DOS date format: year 0, month 1, day 1. */
const dosDate = 0x0021;
/** Regular file, mode 0644, in the high half of the external attributes of a Unix entry. */
const unixMode = 0o100644;
/** Version 2.0 (needed to read) with the Unix host (made by). */
const versionNeeded = 20;
const versionMadeBy = (3 << 8) | versionNeeded;

const crcTable = (() => {
  const table = new Uint32Array(256);
  for (let n = 0; n < 256; n += 1) {
    let c = n;
    for (let k = 0; k < 8; k += 1) {
      c = (c & 1) !== 0 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    }
    table[n] = c >>> 0;
  }
  return table;
})();

/** The CRC-32 (IEEE) of `bytes`, as ZIP stores it. */
export function crc32(bytes: Uint8Array): number {
  let c = 0xffffffff;
  for (const b of bytes) {
    c = (crcTable[(c ^ b) & 0xff] ?? 0) ^ (c >>> 8);
  }
  return (c ^ 0xffffffff) >>> 0;
}

async function sha256Hex(bytes: Uint8Array<ArrayBuffer>): Promise<string> {
  const digest = new Uint8Array(await globalThis.crypto.subtle.digest("SHA-256", bytes));
  return Array.from(digest, (b) => b.toString(16).padStart(2, "0")).join("");
}

/** The code-unit order Go's `strings.Compare` gives for the ASCII names of a project. */
function byPath(a: ArchiveFile, b: ArchiveFile): number {
  return a.path < b.path ? -1 : a.path > b.path ? 1 : 0;
}

interface Entry {
  readonly name: Uint8Array;
  readonly data: Uint8Array;
  readonly crc: number;
  offset: number;
}

function zip(entries: readonly { readonly name: string; readonly data: Uint8Array }[]): Uint8Array<ArrayBuffer> {
  const encoder = new TextEncoder();
  const prepared: Entry[] = entries.map((e) => ({
    name: encoder.encode(e.name),
    data: e.data,
    crc: crc32(e.data),
    offset: 0,
  }));
  const localSize = prepared.reduce((n, e) => n + 30 + e.name.length + e.data.length, 0);
  const centralSize = prepared.reduce((n, e) => n + 46 + e.name.length, 0);
  const out = new Uint8Array(localSize + centralSize + 22);
  const view = new DataView(out.buffer);
  let at = 0;
  for (const e of prepared) {
    e.offset = at;
    view.setUint32(at, 0x04034b50, true);
    view.setUint16(at + 4, versionNeeded, true);
    view.setUint16(at + 6, 0, true);
    view.setUint16(at + 8, 0, true);
    view.setUint16(at + 10, 0, true);
    view.setUint16(at + 12, dosDate, true);
    view.setUint32(at + 14, e.crc, true);
    view.setUint32(at + 18, e.data.length, true);
    view.setUint32(at + 22, e.data.length, true);
    view.setUint16(at + 26, e.name.length, true);
    view.setUint16(at + 28, 0, true);
    out.set(e.name, at + 30);
    out.set(e.data, at + 30 + e.name.length);
    at += 30 + e.name.length + e.data.length;
  }
  const centralStart = at;
  for (const e of prepared) {
    view.setUint32(at, 0x02014b50, true);
    view.setUint16(at + 4, versionMadeBy, true);
    view.setUint16(at + 6, versionNeeded, true);
    view.setUint16(at + 8, 0, true);
    view.setUint16(at + 10, 0, true);
    view.setUint16(at + 12, 0, true);
    view.setUint16(at + 14, dosDate, true);
    view.setUint32(at + 16, e.crc, true);
    view.setUint32(at + 20, e.data.length, true);
    view.setUint32(at + 24, e.data.length, true);
    view.setUint16(at + 28, e.name.length, true);
    view.setUint16(at + 30, 0, true);
    view.setUint16(at + 32, 0, true);
    view.setUint16(at + 34, 0, true);
    view.setUint16(at + 36, 0, true);
    view.setUint32(at + 38, (unixMode << 16) >>> 0, true);
    view.setUint32(at + 42, e.offset, true);
    out.set(e.name, at + 46);
    at += 46 + e.name.length;
  }
  view.setUint32(at, 0x06054b50, true);
  view.setUint16(at + 8, prepared.length, true);
  view.setUint16(at + 10, prepared.length, true);
  view.setUint32(at + 12, centralSize, true);
  view.setUint32(at + 16, centralStart, true);
  return out;
}

/** The archive of `files` for `app`: the manifest first, then every file under `project/`, sorted by path. */
export async function packArchive(app: ArchiveApp, files: readonly ArchiveFile[]): Promise<Uint8Array<ArrayBuffer>> {
  const sorted = [...files].sort(byPath);
  const listed = await Promise.all(
    sorted.map(async (f) => ({
      path: f.path,
      size: f.content.length,
      sha256: await sha256Hex(new Uint8Array(f.content)),
    })),
  );
  const manifest = JSON.stringify(
    { format: "plux-archive", version: 1, app: { ...app, tags: [...app.tags] }, files: listed },
    null,
    2,
  );
  const encoder = new TextEncoder();
  return zip([
    { name: manifestName, data: encoder.encode(`${manifest}\n`) },
    ...sorted.map((f) => ({ name: `${projectPrefix}${f.path}`, data: f.content })),
  ]);
}

export interface ZipSummary {
  /** The number of entries the central directory lists. */
  readonly entries: number;
  /** The sum of the sizes the entries declare after unzipping. */
  readonly expandedBytes: number;
}

/**
 * The entry count and declared expanded size of a ZIP file, read from its central directory without unzipping.
 * `undefined` when the bytes are not a ZIP file. The numbers only let Studio refuse an archive no limit allows
 * before sending it; the server counts the bytes it actually reads.
 */
export function zipSummary(bytes: Uint8Array): ZipSummary | undefined {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  let end = bytes.length - 22;
  const lowest = Math.max(0, bytes.length - 22 - 0xffff);
  while (end >= lowest && view.getUint32(end, true) !== 0x06054b50) {
    end -= 1;
  }
  if (end < lowest) {
    return undefined;
  }
  const entries = view.getUint16(end + 10, true);
  let at = view.getUint32(end + 16, true);
  let expandedBytes = 0;
  for (let i = 0; i < entries; i += 1) {
    if (at + 46 > bytes.length || view.getUint32(at, true) !== 0x02014b50) {
      return undefined;
    }
    expandedBytes += view.getUint32(at + 24, true);
    at += 46 + view.getUint16(at + 28, true) + view.getUint16(at + 30, true) + view.getUint16(at + 32, true);
  }
  return { entries, expandedBytes };
}
