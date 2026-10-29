// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Button } from "./button.tsx";
import type { CaseDefinition } from "./case.ts";
import { caseText } from "./case.ts";
import { notify, Toaster } from "./toast.tsx";

export const cases: readonly CaseDefinition[] = [
  {
    name: "success and error",
    element: (
      <>
        <Toaster direction="ltr" regionLabel={caseText.regionLabel} />
        <Button onClick={() => notify("success", caseText.save)}>{caseText.save}</Button>
        <Button variant="danger" onClick={() => notify("error", caseText.required)}>
          {caseText.delete}
        </Button>
      </>
    ),
  },
];
