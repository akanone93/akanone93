// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Translation keys in PXL (I18N-007): `t(keyId, args)` names a key by its identifier, as a document's `{"$t": id}`
 * does, so renaming a key never breaks an expression. These helpers let the editor offer the keys by name inside
 * `t("…")` and show the name of the key each identifier is.
 */

import { keyCalls } from "../localisation/index.ts";

export { keyCalls };

/** A translation key as the editor offers it. */
export interface PxlKey {
  readonly id: string;
  readonly key: string;
}

/**
 * The string literal that is `t(`'s first argument around `pos`, where a key belongs: the span of its content up to
 * the closing quote, if there is one. Undefined outside such a literal.
 */
export function keyLiteralAt(text: string, pos: number): { readonly from: number; readonly to: number } | undefined {
  const before = text.slice(0, pos);
  const open = /\bt\(\s*(["'])([^"'\\]*)$/.exec(before);
  if (open === null) {
    return undefined;
  }
  const from = pos - (open[2] ?? "").length;
  const close = text.indexOf(open[1] ?? '"', pos);
  const rest = close < 0 ? text.slice(pos) : text.slice(pos, close);
  return /^[^"'\\]*$/.test(rest) ? { from, to: pos + rest.length } : undefined;
}

/** The keys whose name or identifier holds `typed`, by name. */
export function matchingKeys(keys: readonly PxlKey[], typed: string): PxlKey[] {
  const q = typed.toLowerCase();
  return keys
    .filter((k) => q === "" || k.key.toLowerCase().includes(q) || k.id.toLowerCase().startsWith(q))
    .sort((a, b) => a.key.localeCompare(b.key));
}
