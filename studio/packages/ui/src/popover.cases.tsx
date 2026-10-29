// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Button } from "./button.tsx";
import type { CaseDefinition } from "./case.ts";
import { caseText } from "./case.ts";
import { Popover } from "./popover.tsx";

export const cases: readonly CaseDefinition[] = [
  {
    name: "closed",
    element: (
      <Popover trigger={<Button variant="secondary">{caseText.details}</Button>} title={caseText.details}>
        <p>{caseText.body}</p>
      </Popover>
    ),
  },
  {
    name: "open",
    element: (
      <Popover defaultOpen trigger={<Button variant="secondary">{caseText.details}</Button>} title={caseText.details}>
        <p>{caseText.longText}</p>
      </Popover>
    ),
  },
];
