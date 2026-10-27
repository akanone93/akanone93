// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

/** A10 search: `?env`. */
export const functionsEnvSearchSchema = z.object({ env: z.string().optional().catch(undefined) });

export const functionTabs = ["versions", "assurance", "capabilities", "tests", "metrics", "logs"] as const;
export type FunctionTab = (typeof functionTabs)[number];

/** A11 search: `?tab&env`. A bad tab falls back to the versions. */
export const functionSearchSchema = z.object({
  tab: z.enum(functionTabs).optional().catch(undefined),
  env: z.string().optional().catch(undefined),
});

/** The window the metrics tab asks for, in seconds. */
export const metricsWindowSeconds = 3600;
