// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { LiveChip } from "./live-chip.tsx";

export const cases: readonly CaseDefinition[] = [
  { name: "device", element: <LiveChip label="Pixel 9 · live · 41 ms" /> },
];
