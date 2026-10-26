// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { lazy } from "react";

/** React Flow, its stylesheet and the step forms load when the first graph opens, never with the app (STU-122). */
export const GraphCanvas = lazy(() => import("./graph-canvas.tsx"));
