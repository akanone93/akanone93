// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement, ReactNode } from "react";
import { Tooltip as ShadcnTooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./components/tooltip";

/** The tooltip opens after 400 ms. */
export const tooltipDelayMs = 400;

export interface TooltipProps {
  /** Caption from the message catalogue; without one there is no tooltip and the child is rendered as it is. */
  readonly content: string | undefined;
  /** The focusable element the tooltip describes. */
  readonly children: ReactElement;
  /** The side the tooltip opens on; above when absent. */
  readonly side?: "top" | "bottom" | "inline-start" | "inline-end";
  /**
   * The child may be disabled: a disabled control takes no pointer or focus, so the tooltip hangs on a focusable span
   * around it instead (a disabled action's reason, #220).
   */
  readonly disabledTrigger?: boolean;
}

/**
 * Studio's one tooltip (ADR-0103, #220): shadcn's Tooltip, shown on hover and keyboard focus, dismissed by Escape
 * (STU-011). Studio never uses the browser's own `title` tooltip on a control, which looked different and, on a control
 * that also had this one, showed two at once; a test keeps it that way. The app's root holds the one
 * `TooltipProviders` that groups them, so moving from one control to the next opens the next tooltip at once.
 */
export function Tooltip({ content, children, side, disabledTrigger }: TooltipProps): ReactElement {
  if (content === undefined || content === "") {
    return children;
  }
  const trigger = disabledTrigger ? (
    // biome-ignore lint/a11y/noNoninteractiveTabindex: the span carries the disabled control's tooltip to the keyboard
    <span className="inline-flex" tabIndex={0}>
      {children}
    </span>
  ) : (
    children
  );
  return (
    <ShadcnTooltip>
      <TooltipTrigger delay={tooltipDelayMs} render={trigger} />
      <TooltipContent {...(side === undefined ? {} : { side })}>{content}</TooltipContent>
    </ShadcnTooltip>
  );
}

/** The provider the app's root holds: every tooltip of the page shares its delay group (#220). */
export function TooltipProviders({ children }: { readonly children: ReactNode }): ReactElement {
  return <TooltipProvider delay={tooltipDelayMs}>{children}</TooltipProvider>;
}
