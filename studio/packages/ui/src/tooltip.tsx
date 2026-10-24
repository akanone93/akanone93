// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Tooltip as BaseTooltip } from "@base-ui/react/tooltip";
import type { ReactElement } from "react";

/** U11: the tooltip opens after 400 ms. */
export const tooltipDelayMs = 400;

export interface TooltipProps {
  /** Caption from the message catalogue. */
  readonly content: string;
  /** The focusable element the tooltip describes. */
  readonly children: ReactElement;
}

/** Navy caption shown on hover and keyboard focus; Escape dismisses it (STU-011). */
export function Tooltip({ content, children }: TooltipProps): ReactElement {
  return (
    <BaseTooltip.Provider delay={tooltipDelayMs}>
      <BaseTooltip.Root>
        <BaseTooltip.Trigger render={children} />
        <BaseTooltip.Portal>
          <BaseTooltip.Positioner sideOffset={6}>
            <BaseTooltip.Popup className="rounded-md bg-brand-ink/94 px-2 py-1 text-caption text-on-accent">
              {content}
            </BaseTooltip.Popup>
          </BaseTooltip.Positioner>
        </BaseTooltip.Portal>
      </BaseTooltip.Root>
    </BaseTooltip.Provider>
  );
}
