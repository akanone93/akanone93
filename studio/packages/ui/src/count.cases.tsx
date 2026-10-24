// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { Count } from "./count.tsx";

export const cases: readonly CaseDefinition[] = [
  { name: "neutral", element: <Count value={4} label="4 pages" /> },
  { name: "warning", element: <Count value={2} tone="warning" label="2 warnings" /> },
  { name: "large", element: <Count value={128} label="128 notifications" /> },
];
