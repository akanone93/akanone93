// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Icon, type IconName } from "@plux/ui";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@plux/ui/components/collapsible";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@plux/ui/components/tabs";
import type { ReactElement, ReactNode } from "react";

/** One pane of a dock: its tab on the rail (icon, short name) and what it shows. */
export interface Pane {
  readonly value: string;
  readonly label: string;
  readonly icon: IconName;
  /** A short figure on the pane's tab, such as how many screens or problems it lists. */
  readonly meta?: string;
  /** Controls that stay at the top of the pane while its body scrolls (search, filters). */
  readonly toolbar?: ReactNode;
  readonly panel: ReactNode;
}

/**
 * A dock as a design tool lays it out: a bold rail on the primary colour along the dock's start (the editor's mark on
 * top, a labelled tab per pane, the open one raised in white), and beside it the dock's header (which editor, what it
 * edits) over the open pane. Arrow keys move along the rail.
 */
export function DockTabs({
  panes,
  value,
  defaultValue,
  onValueChange,
  mark,
  head,
}: {
  readonly panes: readonly Pane[];
  readonly value?: string;
  readonly defaultValue: string;
  readonly onValueChange?: (value: string) => void;
  /** The editor's mark at the top of the rail. */
  readonly mark: IconName;
  /** The dock's header: the editor's name and what it edits. */
  readonly head: ReactNode;
}): ReactElement {
  return (
    <Tabs
      {...(value === undefined ? { defaultValue } : { value })}
      onValueChange={(v) => onValueChange?.(String(v))}
      orientation="vertical"
      className="dock-tabs"
    >
      <div className="dock-rail-column">
        <span className="dock-mark" aria-hidden="true">
          <Icon name={mark} />
        </span>
        <TabsList variant="line" className="dock-rail">
          {panes.map((p) => (
            <TabsTrigger key={p.value} value={p.value} className="dock-rail-tab">
              <Icon name={p.icon} />
              <span className="dock-rail-label">{p.label}</span>
              {p.meta !== undefined && p.meta !== "0" && (
                <span className="dock-rail-meta" aria-hidden="true">
                  {p.meta}
                </span>
              )}
            </TabsTrigger>
          ))}
        </TabsList>
      </div>
      <div className="dock-main">
        <header className="dock-head">{head}</header>
        {panes.map((p) => (
          <TabsContent key={p.value} value={p.value} className="dock-pane">
            {p.toolbar !== undefined && <div className="dock-pane-toolbar">{p.toolbar}</div>}
            <div className="dock-pane-body">{p.panel}</div>
          </TabsContent>
        ))}
      </div>
    </Tabs>
  );
}

/** A section of a pane that folds away: its icon, name, count and an optional action on the header line. */
export function DockSection({
  label,
  icon,
  count,
  defaultOpen = true,
  action,
  children,
}: {
  readonly label: string;
  readonly icon: IconName;
  readonly count?: number;
  readonly defaultOpen?: boolean;
  readonly action?: ReactNode;
  readonly children: ReactNode;
}): ReactElement {
  return (
    <Collapsible defaultOpen={defaultOpen} className="dock-section">
      <div className="dock-section-head">
        <CollapsibleTrigger className="dock-section-trigger">
          <Icon name="caret-down" size="pill" className="dock-section-caret" />
          <Icon name={icon} size="pill" />
          <span className="min-w-0 flex-1 truncate text-start">{label}</span>
          {count !== undefined && <span className="dock-section-count">{count}</span>}
        </CollapsibleTrigger>
        {action}
      </div>
      <CollapsibleContent className="dock-section-body">{children}</CollapsibleContent>
    </Collapsible>
  );
}
