// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { lazy } from "react";
import { installMonacoEnvironment } from "./monaco-env.ts";

/**
 * Monaco is several megabytes: its chunk loads when the first overlay opens, never with the app (STU-122).
 * Its environment (workers, Trusted Types policies) is installed first, because Monaco reads it while its
 * modules evaluate.
 */
export const MonacoView = lazy(() => {
  installMonacoEnvironment();
  return import("./monaco-view.tsx");
});
