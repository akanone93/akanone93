// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { crc32 } from "./plux-archive.ts";

export interface ZipEntry {
  readonly name: string;
  readonly data: Uint8Array;
  /** The raw header fields the format promises (ADR-0102). */
  readonly method: number;
  readonly dosTime: number;
  readonly dosDate: number;
  readonly mode: number;
}

/** Reads a stored ZIP by its central directory and checks every CRC; for tests of the archive writer. */
export function readZip(bytes: Uint8Array): ZipEntry[] {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const end = bytes.length - 22;
  if (view.getUint32(end, true) !== 0x06054b50) {
    throw new Error("no end of central directory");
  }
  const count = view.getUint16(end + 10, true);
  let at = view.getUint32(end + 16, true);
  const out: ZipEntry[] = [];
  for (let i = 0; i < count; i += 1) {
    const method = view.getUint16(at + 10, true);
    const dosTime = view.getUint16(at + 12, true);
    const dosDate = view.getUint16(at + 14, true);
    const crc = view.getUint32(at + 16, true);
    const size = view.getUint32(at + 24, true);
    const nameLength = view.getUint16(at + 28, true);
    const mode = view.getUint32(at + 38, true) >>> 16;
    const offset = view.getUint32(at + 42, true);
    const name = new TextDecoder().decode(bytes.subarray(at + 46, at + 46 + nameLength));
    const start = offset + 30 + view.getUint16(offset + 26, true) + view.getUint16(offset + 28, true);
    const data = bytes.subarray(start, start + size);
    if (crc32(data) !== crc) {
      throw new Error(`bad CRC for ${name}`);
    }
    out.push({ name, data, method, dosTime, dosDate, mode });
    at += 46 + nameLength + view.getUint16(at + 30, true) + view.getUint16(at + 32, true);
  }
  return out;
}

/** The JSON of one entry of a read archive. */
export function jsonOf(entries: readonly ZipEntry[], name: string): Record<string, unknown> {
  const found = entries.find((e) => e.name === name);
  if (found === undefined) {
    throw new Error(`no entry ${name}`);
  }
  return JSON.parse(new TextDecoder().decode(found.data)) as Record<string, unknown>;
}
