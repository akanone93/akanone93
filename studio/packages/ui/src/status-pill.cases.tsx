// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { StatusPill } from "./status-pill.tsx";

export const cases: readonly CaseDefinition[] = [
  { name: "success", element: <StatusPill status="success" label="Passing" value="42 tests" /> },
  { name: "warning", element: <StatusPill status="warning" label="Slow" value="18 ms" /> },
  { name: "danger", element: <StatusPill status="danger" label="Failing" /> },
  { name: "neutral", element: <StatusPill status="neutral" label="Idle" /> },
];
