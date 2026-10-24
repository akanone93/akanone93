// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { caseText } from "./case.ts";
import { FloatingPanel, GlassCapsule } from "./glass.tsx";

export const cases: readonly CaseDefinition[] = [
  { name: "capsule", element: <GlassCapsule label={caseText.actions}>{caseText.pages}</GlassCapsule> },
  { name: "floating panel", element: <FloatingPanel label={caseText.details}>{caseText.body}</FloatingPanel> },
];
