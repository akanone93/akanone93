// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Icon, type IconName, tooltipDelayMs } from "@plux/ui";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@plux/ui/components/tabs";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@plux/ui/components/tooltip";

import type { ReactElement, ReactNode } from "react";

/** One pane of a dock: its tab on the rail (icon, name) and what it shows, under its name and an optional count. */
export interface Pane {
  readonly value: string;
  readonly label: string;
  readonly icon: IconName;
  /** A short figure beside the pane's name, such as how many widgets it lists. */
  readonly meta?: string;
  readonly panel: ReactNode;
}

/**
 * A dock of the plugin canvas as a design tool lays it out: a rail of icon tabs along the dock's start, each named by
 * its tooltip and its accessible name, and the open pane beside it under its name. Arrow keys move along the rail.
 */
export function DockTabs({
  panes,
  defaultValue,
}: {
  readonly panes: readonly Pane[];
  readonly defaultValue: string;
}): ReactElement {
  return (
    <TooltipProvider delay={tooltipDelayMs}>
      <Tabs defaultValue={defaultValue} orientation="vertical" className="dock-tabs">
        <TabsList variant="line" className="dock-rail">
          {panes.map((p) => (
            <Tooltip key={p.value}>
              <TooltipTrigger render={<TabsTrigger value={p.value} aria-label={p.label} className="dock-rail-tab" />}>
                <Icon name={p.icon} />
              </TooltipTrigger>
              <TooltipContent side="inline-end">{p.label}</TooltipContent>
            </Tooltip>
          ))}
        </TabsList>
        {panes.map((p) => (
          <TabsContent key={p.value} value={p.value} className="dock-pane">
            <header className="dock-pane-head">
              <h2>{p.label}</h2>
              {p.meta !== undefined && <span className="dock-pane-meta">{p.meta}</span>}
            </header>
            <div className="dock-pane-body">{p.panel}</div>
          </TabsContent>
        ))}
      </Tabs>
    </TooltipProvider>
  );
}
