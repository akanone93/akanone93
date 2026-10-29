// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** The sfnt versions of a TrueType (`0x00010000`, `true`) and an OpenType with CFF outlines (`OTTO`) font. */
const SFNT_VERSIONS: ReadonlySet<number> = new Set([0x00010000, 0x74727565, 0x4f54544f]);
const NAME_TAG = 0x6e616d65;
const HEADER_BYTES = 12;
const DIRECTORY_ENTRY_BYTES = 16;
const NAME_HEADER_BYTES = 6;
const NAME_RECORD_BYTES = 12;
const NAME_FAMILY = 1;
const NAME_TYPOGRAPHIC_FAMILY = 16;
const PLATFORM_MACINTOSH = 1;
const PLATFORM_WINDOWS = 3;
const LANGUAGE_WINDOWS_EN_US = 0x0409;

interface Candidate {
  readonly rank: number;
  readonly family: string;
}

/**
 * The family name a TrueType or OpenType font declares, read from its `name` table, or null when the bytes are
 * not a single font or the table is malformed. The typographic family (name ID 16) wins over the family (name ID
 * 1); a Windows (platform 3) UTF-16BE string wins over a Macintosh (platform 1) one, English over other languages.
 * Every offset and length is checked against the bytes before it is read, so a truncated or hostile file gives
 * null and never an exception; the walk is bounded by the table directory and the record count, both 16-bit.
 */
export function fontFamilyName(bytes: Uint8Array): string | null {
  try {
    return readFamily(bytes);
  } catch {
    return null;
  }
}

function readFamily(bytes: Uint8Array): string | null {
  if (bytes.byteLength < HEADER_BYTES) {
    return null;
  }
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  if (!SFNT_VERSIONS.has(view.getUint32(0))) {
    return null;
  }
  const table = findNameTable(view);
  return table === null ? null : bestFamily(bytes, view, table);
}

/** The offset and length of the `name` table, or null when it is missing or does not lie inside the bytes. */
function findNameTable(view: DataView): { offset: number; length: number } | null {
  const tableCount = view.getUint16(4);
  if (HEADER_BYTES + tableCount * DIRECTORY_ENTRY_BYTES > view.byteLength) {
    return null;
  }
  for (let i = 0; i < tableCount; i++) {
    const entry = HEADER_BYTES + i * DIRECTORY_ENTRY_BYTES;
    if (view.getUint32(entry) !== NAME_TAG) {
      continue;
    }
    const offset = view.getUint32(entry + 8);
    const length = view.getUint32(entry + 12);
    const inside = offset <= view.byteLength && length <= view.byteLength - offset;
    return inside && length >= NAME_HEADER_BYTES ? { offset, length } : null;
  }
  return null;
}

function bestFamily(bytes: Uint8Array, view: DataView, table: { offset: number; length: number }): string | null {
  const { offset, length } = table;
  const count = view.getUint16(offset + 2);
  const storage = view.getUint16(offset + 4);
  if (NAME_HEADER_BYTES + count * NAME_RECORD_BYTES > length || storage > length) {
    return null;
  }
  let best: Candidate | null = null;
  for (let i = 0; i < count; i++) {
    const candidate = readRecord(bytes, view, table, storage, offset + NAME_HEADER_BYTES + i * NAME_RECORD_BYTES);
    if (candidate !== null && (best === null || candidate.rank > best.rank)) {
      best = candidate;
    }
  }
  return best?.family ?? null;
}

/** One name record as a ranked candidate, or null when it is not a family name or lies outside the table. */
function readRecord(
  bytes: Uint8Array,
  view: DataView,
  table: { offset: number; length: number },
  storage: number,
  record: number,
): Candidate | null {
  const platform = view.getUint16(record);
  const language = view.getUint16(record + 4);
  const nameId = view.getUint16(record + 6);
  const size = view.getUint16(record + 8);
  const start = storage + view.getUint16(record + 10);
  const isFamily = nameId === NAME_FAMILY || nameId === NAME_TYPOGRAPHIC_FAMILY;
  const windows = platform === PLATFORM_WINDOWS;
  if (!isFamily || (!windows && platform !== PLATFORM_MACINTOSH) || start + size > table.length) {
    return null;
  }
  const raw = bytes.subarray(table.offset + start, table.offset + start + size);
  const family = windows ? decodeUtf16(raw) : decodeLatin1(raw);
  if (family === null) {
    return null;
  }
  const english = windows ? language === LANGUAGE_WINDOWS_EN_US : language === 0;
  const rank = (nameId === NAME_TYPOGRAPHIC_FAMILY ? 4 : 0) + (windows ? 2 : 0) + (english ? 1 : 0);
  return { rank, family };
}

/** A well-formed name: non-empty after trimming, with no control characters. */
function clean(text: string): string | null {
  const trimmed = text.trim();
  // biome-ignore lint/suspicious/noControlCharactersInRegex: control characters are what is rejected
  return trimmed === "" || /[\u0000-\u001f\u007f]/.test(trimmed) ? null : trimmed;
}

function decodeUtf16(raw: Uint8Array): string | null {
  if (raw.byteLength % 2 !== 0) {
    return null;
  }
  try {
    return clean(new TextDecoder("utf-16be", { fatal: true }).decode(raw));
  } catch {
    return null;
  }
}

function decodeLatin1(raw: Uint8Array): string | null {
  return clean(String.fromCharCode(...raw));
}
