// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import { formatShortcut } from "../../lib/commands/index.ts";

/** A command's shortcut as keys, the way this platform writes them (⌘K on Apple devices, Ctrl+K elsewhere). */
export function ShortcutKey({ shortcut }: { readonly shortcut: string }): ReactElement {
  const apple = typeof navigator !== "undefined" && /mac|iphone|ipad/i.test(navigator.platform);
  return (
    <span aria-hidden="true" className="shell-kbd">
      {formatShortcut(shortcut, apple)}
    </span>
  );
}
