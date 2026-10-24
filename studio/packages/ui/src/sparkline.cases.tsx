// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { Sparkline } from "./sparkline.tsx";

export const cases: readonly CaseDefinition[] = [
  { name: "series", element: <Sparkline label="Frame time, latest 7 ms" values={[9, 8, 11, 7, 6, 8, 7]} /> },
  { name: "flat", element: <Sparkline label="Latency, 41 ms" values={[41, 41, 41]} /> },
  { name: "empty", element: <Sparkline label="No samples yet" values={[]} /> },
];
