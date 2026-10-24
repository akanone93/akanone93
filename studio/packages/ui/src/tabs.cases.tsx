// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { caseText } from "./case.ts";
import { Tabs } from "./tabs.tsx";

const tabs = [
  { value: "pages", label: caseText.pages, panel: <p>{caseText.body}</p> },
  { value: "actions", label: caseText.actions, panel: <p>{caseText.hint}</p> },
  { value: "long", label: caseText.longText, panel: <p>{caseText.longText}</p> },
];

export const cases: readonly CaseDefinition[] = [
  { name: "first selected", element: <Tabs tabs={tabs} defaultValue="pages" /> },
  { name: "second selected", element: <Tabs tabs={tabs} defaultValue="actions" /> },
];
