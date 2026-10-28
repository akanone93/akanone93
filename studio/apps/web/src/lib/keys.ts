// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The form of a key a person chooses for an organisation or an app, and of its display name, as the server
 * checks them (backend/internal/tenancy `keyForm`, `checkName`). The server stays the authority; these checks
 * only spare a round trip.
 */
const keyForm = /^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$/;
const MAX_KEY_LENGTH = 64;
const MAX_NAME_BYTES = 200;

/** True when `key` is 1 to 64 lower-case letters, digits and hyphens, starting and ending with a letter or digit. */
export function isKey(key: string): boolean {
  return keyForm.test(key);
}

/** True when the trimmed `name` is 1 to 200 bytes long (the server counts bytes, not characters). */
export function isName(name: string): boolean {
  const bytes = new TextEncoder().encode(name.trim()).length;
  return bytes >= 1 && bytes <= MAX_NAME_BYTES;
}

/** A key suggested from a name: lower-case ASCII words joined by hyphens, at most 64 characters. */
export function keyFromName(name: string): string {
  return name
    .toLowerCase()
    .normalize("NFKD")
    .replace(/\p{M}/gu, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+/, "")
    .slice(0, MAX_KEY_LENGTH)
    .replace(/-+$/, "");
}
