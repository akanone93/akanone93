// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Button } from "./button.tsx";
import type { CaseDefinition } from "./case.ts";
import { caseText } from "./case.ts";
import { Icon } from "./icon.tsx";
import { Tooltip } from "./tooltip.tsx";

export const cases: readonly CaseDefinition[] = [
  {
    name: "icon button",
    element: (
      <Tooltip content={caseText.tipCaption}>
        <Button variant="ghost" aria-label={caseText.tipCaption}>
          <Icon name="plus" />
        </Button>
      </Tooltip>
    ),
  },
];
