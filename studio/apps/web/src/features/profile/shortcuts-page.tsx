// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Card, CardContent, CardHeader, CardTitle } from "@plux/ui/components/card";
import { Kbd, KbdGroup } from "@plux/ui/components/kbd";
import type { ReactElement } from "react";
import {
  type Command,
  type CommandGroup,
  commandMessages,
  shortcutKeys,
  useCommands,
} from "../../lib/commands/index.ts";
import { ProfileLayout } from "./profile-layout.tsx";

type Messages = ReturnType<typeof commandMessages>;

const groupOrder: readonly CommandGroup[] = ["navigate", "organisation", "appearance", "edit", "screens", "account"];

const groupTitle = (m: Messages, group: CommandGroup): string =>
  ({
    navigate: m.groupNavigate,
    organisation: m.groupOrganisation,
    appearance: m.groupAppearance,
    edit: m.groupEdit,
    screens: m.groupScreens,
    account: m.groupAccount,
  })[group];

/** A shortcut as its keys, one key cap each: Ctrl+Shift+Z reads as three caps. */
export function ShortcutKeys({ shortcut, apple }: { readonly shortcut: string; readonly apple: boolean }) {
  return (
    <KbdGroup>
      {shortcutKeys(shortcut, apple).map((key, i) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: a key may repeat, the order is fixed
        <Kbd key={i}>{key}</Kbd>
      ))}
    </KbdGroup>
  );
}

/** U3: keyboard shortcuts, generated from the command registry, grouped as the command palette groups them. */
export function ShortcutsPage(): ReactElement {
  const m = commandMessages(useLingui().i18n);
  const { registry } = useCommands();
  const apple = typeof navigator !== "undefined" && /mac|iphone|ipad/i.test(navigator.platform);
  const withKeys = registry.commands.filter((c): c is Command & { shortcut: string } => c.shortcut !== undefined);
  return (
    <ProfileLayout id="shortcuts-title" title={m.shortcutsTitle} caption={m.shortcutsCaption}>
      {groupOrder.map((group) => {
        const commands = withKeys.filter((c) => c.group === group);
        if (commands.length === 0) {
          return null;
        }
        return (
          <Card key={group}>
            <CardHeader>
              <CardTitle>
                <h2>{groupTitle(m, group)}</h2>
              </CardTitle>
            </CardHeader>
            <CardContent>
              <table className="w-full text-sm">
                <caption className="sr-only">{groupTitle(m, group)}</caption>
                <tbody className="divide-y">
                  {commands.map((c) => (
                    <tr key={c.id}>
                      <th scope="row" className="py-2.5 pe-4 text-start font-normal">
                        {c.title}
                      </th>
                      <td className="py-2.5 text-end">
                        <ShortcutKeys shortcut={c.shortcut} apple={apple} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </CardContent>
          </Card>
        );
      })}
    </ProfileLayout>
  );
}
