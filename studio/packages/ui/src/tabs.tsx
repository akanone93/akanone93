// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ComponentProps, ReactElement, ReactNode } from "react";
import { Tabs as ShadcnTabs, TabsContent, TabsList, TabsTrigger } from "./components/tabs";

export interface TabDefinition {
  readonly value: string;
  readonly label: string;
  readonly panel: ReactNode;
}

export type TabsProps = Omit<ComponentProps<typeof ShadcnTabs>, "children"> & {
  readonly tabs: readonly TabDefinition[];
};

/** shadcn's Tabs (ADR-0103). Arrow keys follow the reading direction. */
export function Tabs({ tabs, ...root }: TabsProps): ReactElement {
  return (
    <ShadcnTabs {...root}>
      <TabsList>
        {tabs.map((t) => (
          <TabsTrigger key={t.value} value={t.value}>
            {t.label}
          </TabsTrigger>
        ))}
      </TabsList>
      {tabs.map((t) => (
        <TabsContent key={t.value} value={t.value}>
          {t.panel}
        </TabsContent>
      ))}
    </ShadcnTabs>
  );
}
