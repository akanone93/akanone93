// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { MetricStrip } from "./metric-strip.tsx";

export const cases: readonly CaseDefinition[] = [
  {
    name: "three metrics",
    element: (
      <MetricStrip
        metrics={[
          { label: "Bundle", value: "412 KB" },
          { label: "Frame time", value: "7 ms" },
          { label: "Synced", value: "2 min ago" },
        ]}
      />
    ),
  },
];
