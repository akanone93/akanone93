// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Menu as BaseMenu } from "@base-ui/react/menu";
import type { ReactElement } from "react";
import { cx, focusRing } from "./cx.ts";

export type MenuEntry =
  | {
      readonly kind: "item";
      readonly id: string;
      readonly label: string;
      readonly onSelect: () => void;
      readonly danger?: boolean;
      readonly disabled?: boolean;
    }
  | { readonly kind: "separator"; readonly id: string };

export type MenuProps = Omit<BaseMenu.Root.Props, "children"> & {
  readonly trigger: ReactElement;
  readonly entries: readonly MenuEntry[];
};

/** Action menu. Arrow keys follow the reading direction, Home/End and type-ahead come from Base UI (STU-011). */
export function Menu({ trigger, entries, ...root }: MenuProps): ReactElement {
  return (
    <BaseMenu.Root {...root}>
      <BaseMenu.Trigger render={trigger} />
      <BaseMenu.Portal>
        <BaseMenu.Positioner sideOffset={8} className="z-20">
          <BaseMenu.Popup className="flex min-w-40 flex-col rounded-xl bg-glass p-1 shadow-e2">
            {entries.map((e) =>
              e.kind === "separator" ? (
                <BaseMenu.Separator key={e.id} className="my-1 h-px bg-hairline" />
              ) : (
                <BaseMenu.Item
                  key={e.id}
                  onClick={e.onSelect}
                  {...(e.disabled !== undefined && { disabled: e.disabled })}
                  className={cx(
                    "flex h-(--density-field) cursor-default items-center rounded-md px-3 text-body",
                    e.danger === true ? "text-danger" : "text-primary",
                    "data-highlighted:bg-hover data-disabled:opacity-50",
                    focusRing,
                  )}
                >
                  {e.label}
                </BaseMenu.Item>
              ),
            )}
          </BaseMenu.Popup>
        </BaseMenu.Positioner>
      </BaseMenu.Portal>
    </BaseMenu.Root>
  );
}
