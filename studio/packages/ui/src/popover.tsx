// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ComponentProps, ReactElement, ReactNode } from "react";
import {
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
  Popover as ShadcnPopover,
} from "./components/popover";

export type PopoverProps = Omit<ComponentProps<typeof ShadcnPopover>, "children"> & {
  /** The element that toggles the popover. */
  readonly trigger: ReactElement;
  /** A header naming what the popover shows. */
  readonly title?: string;
  readonly children?: ReactNode;
};

/** shadcn's Popover (ADR-0103), anchored to its trigger; Escape and outside press close it (STU-011). */
export function Popover({ trigger, title, children, ...root }: PopoverProps): ReactElement {
  return (
    <ShadcnPopover {...root}>
      <PopoverTrigger render={trigger} />
      <PopoverContent align="end" className="w-80">
        {title !== undefined && (
          <PopoverHeader>
            <PopoverTitle>{title}</PopoverTitle>
          </PopoverHeader>
        )}
        {children}
      </PopoverContent>
    </ShadcnPopover>
  );
}
