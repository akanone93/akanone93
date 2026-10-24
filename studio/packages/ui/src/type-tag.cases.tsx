// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { TypeTag } from "./type-tag.tsx";

export const cases: readonly CaseDefinition[] = [
  { name: "short", element: <TypeTag>Txt</TypeTag> },
  { name: "long is truncated", element: <TypeTag>Container</TypeTag> },
];
