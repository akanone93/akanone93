// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { FileDrop } from "./file-drop.tsx";

const none = () => undefined;

export const cases: readonly CaseDefinition[] = [
  {
    name: "zone",
    element: <FileDrop label="Upload images" hint="Drag files here, or click to choose them" multiple onFiles={none} />,
  },
  { name: "chosen", element: <FileDrop label="Archive file" chosen="shop.plux" hint=".plux" onFiles={none} /> },
  { name: "button", element: <FileDrop label="Import a template" variant="button" onFiles={none} /> },
  { name: "disabled", element: <FileDrop label="Licence file" disabled onFiles={none} /> },
];
