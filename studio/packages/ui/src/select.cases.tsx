// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { caseText } from "./case.ts";
import { Select } from "./select.tsx";

const options = [
  { value: "en", label: caseText.english },
  { value: "ar", label: caseText.arabic },
  { value: "de", label: caseText.longText },
];

export const cases: readonly CaseDefinition[] = [
  {
    name: "placeholder",
    element: <Select label={caseText.placeholder} options={options} placeholder={caseText.placeholder} />,
  },
  { name: "selected", element: <Select label={caseText.placeholder} options={options} defaultValue="ar" /> },
  { name: "disabled", element: <Select label={caseText.placeholder} options={options} defaultValue="en" disabled /> },
  { name: "long option", element: <Select label={caseText.placeholder} options={options} defaultValue="de" /> },
];
