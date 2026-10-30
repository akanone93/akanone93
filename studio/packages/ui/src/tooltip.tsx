// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import { Tooltip as ShadcnTooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./components/tooltip";

/** The tooltip opens after 400 ms. */
export const tooltipDelayMs = 400;

export interface TooltipProps {
  /** Caption from the message catalogue. */
  readonly content: string;
  /** The focusable element the tooltip describes. */
  readonly children: ReactElement;
}

/** shadcn's Tooltip (ADR-0103), shown on hover and keyboard focus; Escape dismisses it (STU-011). */
export function Tooltip({ content, children }: TooltipProps): ReactElement {
  return (
    <TooltipProvider delay={tooltipDelayMs}>
      <ShadcnTooltip>
        <TooltipTrigger render={children} />
        <TooltipContent>{content}</TooltipContent>
      </ShadcnTooltip>
    </TooltipProvider>
  );
}
