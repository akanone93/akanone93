// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Button } from "./button.tsx";
import type { CaseDefinition } from "./case.ts";
import { caseText } from "./case.ts";
import { AlertDialog, Dialog } from "./dialog.tsx";

const noop = (): void => undefined;

export const cases: readonly CaseDefinition[] = [
  {
    name: "closed with trigger",
    element: (
      <Dialog title={caseText.details} labels={{ close: caseText.close }} trigger={<Button>{caseText.open}</Button>}>
        <p>{caseText.body}</p>
      </Dialog>
    ),
  },
  {
    name: "open",
    element: (
      <Dialog defaultOpen title={caseText.details} description={caseText.hint} labels={{ close: caseText.close }}>
        <p>{caseText.body}</p>
      </Dialog>
    ),
  },
  {
    name: "open, long title",
    element: <Dialog defaultOpen size="sm" title={caseText.longText} labels={{ close: caseText.close }} />,
  },
  {
    name: "alert dialog",
    element: (
      <AlertDialog
        defaultOpen
        title={caseText.title}
        description={caseText.body}
        labels={{ cancel: caseText.cancel, confirm: caseText.confirm }}
        onConfirm={noop}
      />
    ),
  },
];
