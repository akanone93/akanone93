// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { IconName } from "./icons.gen.ts";

/**
 * Icons that carry a direction and mirror under `dir="rtl"` (studio-development 8.3). Phosphor ships no mirror
 * metadata, so this list is kept by hand; every other icon never mirrors (play, search, check, clocks, media).
 */
export const mirroredIcons: ReadonlySet<IconName> = new Set<IconName>([
  "arrow-bend-up-left",
  "arrow-clockwise",
  "arrow-counter-clockwise",
  "arrow-left",
  "arrow-right",
  "caret-left",
  "caret-right",
  "paper-plane-tilt",
  "sign-out",
]);

export function iconMirrors(name: IconName): boolean {
  return mirroredIcons.has(name);
}
