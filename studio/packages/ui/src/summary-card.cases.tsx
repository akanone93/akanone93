// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { SummaryCard } from "./summary-card.tsx";

export const cases: readonly CaseDefinition[] = [
  {
    name: "screens links ghost",
    element: (
      <SummaryCard
        cells={[
          { value: "12", caption: "screens" },
          { value: "31", caption: "links" },
          { value: "0", caption: "ghost" },
        ]}
      />
    ),
  },
];
