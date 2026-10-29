// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Button } from "./button.tsx";
import type { CaseDefinition } from "./case.ts";
import { caseText } from "./case.ts";
import { Icon } from "./icon.tsx";

export const cases: readonly CaseDefinition[] = [
  { name: "primary", element: <Button>{caseText.save}</Button> },
  { name: "secondary", element: <Button variant="secondary">{caseText.cancel}</Button> },
  {
    name: "ghost icon button",
    element: (
      <Button variant="ghost" aria-label={caseText.close}>
        <Icon name="x" />
      </Button>
    ),
  },
  { name: "danger", element: <Button variant="danger">{caseText.delete}</Button> },
  { name: "small", element: <Button size="sm">{caseText.save}</Button> },
  {
    name: "loading",
    element: (
      <Button loading loadingLabel={caseText.loading}>
        {caseText.save}
      </Button>
    ),
  },
  { name: "disabled", element: <Button disabled>{caseText.save}</Button> },
  { name: "long text", element: <Button>{caseText.longText}</Button> },
];
