// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** A UUIDv7 in canonical lower-case form (SCH-002): the millisecond clock in the first 48 bits, random bits after. */
export function uuid7(now: number = Date.now(), random: (n: number) => Uint8Array = randomBytes): string {
  const b = random(16);
  const ts = BigInt(now);
  for (let i = 0; i < 6; i += 1) {
    b[i] = Number((ts >> BigInt(8 * (5 - i))) & 0xffn);
  }
  b[6] = ((b[6] ?? 0) & 0x0f) | 0x70;
  b[8] = ((b[8] ?? 0) & 0x3f) | 0x80;
  const hex = [...b].map((x) => x.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

function randomBytes(n: number): Uint8Array {
  return crypto.getRandomValues(new Uint8Array(n));
}
