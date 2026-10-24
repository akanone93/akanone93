// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Popover as BasePopover } from "@base-ui/react/popover";
import type { ReactElement, ReactNode } from "react";

export type PopoverProps = Omit<BasePopover.Root.Props, "children"> & {
  /** The element that toggles the popover. */
  readonly trigger: ReactElement;
  /** Serif header, where the popover names something (U11). */
  readonly title?: string;
  readonly children?: ReactNode;
};

/** Glass popover anchored to its trigger; Escape and outside press close it (STU-011). */
export function Popover({ trigger, title, children, ...root }: PopoverProps): ReactElement {
  return (
    <BasePopover.Root {...root}>
      <BasePopover.Trigger render={trigger} />
      <BasePopover.Portal>
        <BasePopover.Positioner sideOffset={8}>
          <BasePopover.Popup className="flex flex-col gap-2 rounded-xl bg-glass p-4 shadow-e2">
            {title !== undefined && (
              <BasePopover.Title className="font-serif text-section text-primary">{title}</BasePopover.Title>
            )}
            {children}
          </BasePopover.Popup>
        </BasePopover.Positioner>
      </BasePopover.Portal>
    </BasePopover.Root>
  );
}
