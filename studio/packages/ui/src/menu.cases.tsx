// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Button } from "./button.tsx";
import type { CaseDefinition } from "./case.ts";
import { caseText } from "./case.ts";
import { Menu, type MenuEntry } from "./menu.tsx";

const noop = (): void => undefined;

const entries: readonly MenuEntry[] = [
  { kind: "item", id: "rename", label: caseText.rename, onSelect: noop },
  { kind: "item", id: "duplicate", label: caseText.duplicate, onSelect: noop, disabled: true },
  { kind: "separator", id: "sep" },
  { kind: "item", id: "delete", label: caseText.delete, onSelect: noop, danger: true },
];

export const cases: readonly CaseDefinition[] = [
  {
    name: "closed",
    element: <Menu trigger={<Button variant="secondary">{caseText.actions}</Button>} entries={entries} />,
  },
  {
    name: "open",
    element: <Menu defaultOpen trigger={<Button variant="secondary">{caseText.actions}</Button>} entries={entries} />,
  },
];
