// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Tabs as BaseTabs } from "@base-ui/react/tabs";
import type { ReactElement, ReactNode } from "react";
import { cx, focusRing } from "./cx.ts";

export interface TabDefinition {
  readonly value: string;
  readonly label: string;
  readonly panel: ReactNode;
}

export type TabsProps = Omit<BaseTabs.Root.Props, "children"> & {
  readonly tabs: readonly TabDefinition[];
};

/** Pill tabs: a track on the hover surface, the selected tab a raised pill (U11). Arrow keys follow reading direction. */
export function Tabs({ tabs, ...root }: TabsProps): ReactElement {
  return (
    <BaseTabs.Root {...root}>
      <BaseTabs.List className="inline-flex h-(--density-tab-track) items-center gap-0.5 rounded-full bg-hover p-0.5">
        {tabs.map((t) => (
          <BaseTabs.Tab
            key={t.value}
            value={t.value}
            className={cx(
              "h-full rounded-full px-3 text-label font-medium text-secondary",
              "data-selected:bg-raised data-selected:text-primary data-selected:shadow-e1",
              focusRing,
            )}
          >
            {t.label}
          </BaseTabs.Tab>
        ))}
      </BaseTabs.List>
      {tabs.map((t) => (
        <BaseTabs.Panel key={t.value} value={t.value} className="pt-3">
          {t.panel}
        </BaseTabs.Panel>
      ))}
    </BaseTabs.Root>
  );
}
