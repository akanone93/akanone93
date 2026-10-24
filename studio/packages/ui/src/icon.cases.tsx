// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { caseText } from "./case.ts";
import { Icon } from "./icon.tsx";
import { iconNames } from "./icons.gen.ts";

export const cases: readonly CaseDefinition[] = [
  {
    name: "sizes",
    element: (
      <div className="flex items-center gap-2 text-secondary">
        <Icon name="gear" size="pill" />
        <Icon name="gear" size="tab" />
        <Icon name="gear" size="capsule" />
      </div>
    ),
  },
  { name: "labelled", element: <Icon name="info" label={caseText.details} /> },
  { name: "mirrors in RTL", element: <Icon name="arrow-left" /> },
  { name: "never mirrors", element: <Icon name="play" /> },
  {
    name: "all icons",
    element: (
      <div className="flex flex-wrap gap-2 text-secondary">
        {iconNames.map((n) => (
          <Icon key={n} name={n} />
        ))}
      </div>
    ),
  },
];
