// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { Meter } from "./meter.tsx";

export const cases: readonly CaseDefinition[] = [
  { name: "half", element: <Meter label="Bundle size" value={256} max={512} valueText="256 of 512 KB" /> },
  { name: "empty", element: <Meter label="Limits" value={0} max={100} valueText="0 of 100" /> },
  {
    name: "over budget is clamped",
    element: <Meter label="Bundle size" value={900} max={512} valueText="900 of 512 KB" />,
  },
];
