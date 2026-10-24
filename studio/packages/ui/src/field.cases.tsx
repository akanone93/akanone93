// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { caseText } from "./case.ts";
import { Field, Input, Textarea } from "./field.tsx";

export const cases: readonly CaseDefinition[] = [
  {
    name: "input with description",
    element: (
      <Field label={caseText.name} description={caseText.hint}>
        <Input />
      </Field>
    ),
  },
  {
    name: "input with error",
    element: (
      <Field label={caseText.name} error={caseText.required}>
        <Input />
      </Field>
    ),
  },
  {
    name: "textarea",
    element: (
      <Field label={caseText.details}>
        <Textarea rows={3} />
      </Field>
    ),
  },
  {
    name: "disabled",
    element: (
      <Field label={caseText.name}>
        <Input disabled />
      </Field>
    ),
  },
  {
    name: "long label",
    element: (
      <Field label={caseText.longText}>
        <Input />
      </Field>
    ),
  },
];
