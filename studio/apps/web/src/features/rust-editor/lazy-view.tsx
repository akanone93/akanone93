// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { lazy } from "react";
import { installMonacoEnvironment } from "../json-editor/monaco-env.ts";

/** Monaco loads with the first overlay that needs it, never with the app (STU-122); its environment comes first. */
export const RustMonacoView = lazy(() => {
  installMonacoEnvironment();
  return import("./rust-monaco-view.tsx");
});
