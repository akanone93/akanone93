// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { commandMessages, formatShortcut, useCommands } from "../../lib/commands/index.ts";

/** U3: keyboard shortcuts, generated from the command registry. */
export function ShortcutsPage() {
  const { i18n } = useLingui();
  const m = commandMessages(i18n);
  const { registry } = useCommands();
  const apple = typeof navigator !== "undefined" && /mac|iphone|ipad/i.test(navigator.platform);
  return (
    <section aria-labelledby="shortcuts-title" className="flex max-w-prose flex-col gap-6">
      <header>
        <h1 id="shortcuts-title" className="text-screen">
          {m.shortcutsTitle}
        </h1>
        <p className="text-body text-secondary">{m.shortcutsCaption}</p>
      </header>
      <table>
        <tbody>
          {registry.commands
            .filter((c) => c.shortcut !== undefined)
            .map((c) => (
              <tr key={c.id}>
                <th scope="row" className="py-1 pe-4 text-start text-body font-normal">
                  {c.title}
                </th>
                <td>
                  <kbd className="font-mono text-caption">
                    {c.shortcut === undefined ? m.none : formatShortcut(c.shortcut, apple)}
                  </kbd>
                </td>
              </tr>
            ))}
        </tbody>
      </table>
    </section>
  );
}
