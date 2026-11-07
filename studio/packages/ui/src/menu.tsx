// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ComponentProps, ReactElement } from "react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "./components/dropdown-menu";
import { Icon } from "./icon.tsx";
import type { IconName } from "./icons.ts";

export type MenuEntry =
  | {
      readonly kind: "item";
      readonly id: string;
      readonly label: string;
      readonly onSelect: () => void;
      readonly danger?: boolean;
      readonly disabled?: boolean;
      /** A decorative icon before the label; the label alone names the item. */
      readonly icon?: IconName;
    }
  | { readonly kind: "separator"; readonly id: string };

export type MenuProps = Omit<ComponentProps<typeof DropdownMenu>, "children"> & {
  readonly trigger: ReactElement;
  readonly entries: readonly MenuEntry[];
};

/** Groups the entries between separators, as shadcn's menus expect items inside a group. */
function groups(entries: readonly MenuEntry[]) {
  const out: { id: string; items: Extract<MenuEntry, { kind: "item" }>[] }[] = [];
  for (const e of entries) {
    if (e.kind === "separator") {
      out.push({ id: e.id, items: [] });
    } else if (out.length === 0) {
      out.push({ id: e.id, items: [e] });
    } else {
      out[out.length - 1]?.items.push(e);
    }
  }
  return out;
}

/**
 * shadcn's DropdownMenu (ADR-0103). Arrow keys follow the reading direction; Home, End and type-ahead come from Base
 * UI. It is not modal, so the page keeps its scroll bar while the menu is open (STU-011).
 */
export function Menu({ trigger, entries, ...root }: MenuProps): ReactElement {
  const parts = groups(entries);
  return (
    <DropdownMenu modal={false} {...root}>
      <DropdownMenuTrigger render={trigger} />
      <DropdownMenuContent align="end" className="w-max min-w-40 max-w-80">
        {parts.map((g, i) => [
          i > 0 && <DropdownMenuSeparator key={`${g.id}-separator`} />,
          g.items.length > 0 && (
            <DropdownMenuGroup key={g.id}>
              {g.items.map((e) => (
                <DropdownMenuItem
                  key={e.id}
                  onClick={e.onSelect}
                  {...(e.danger === true && { variant: "destructive" as const })}
                  {...(e.disabled !== undefined && { disabled: e.disabled })}
                >
                  {e.icon !== undefined && <Icon name={e.icon} />}
                  {e.label}
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
          ),
        ])}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
