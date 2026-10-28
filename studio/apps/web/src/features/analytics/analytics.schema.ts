// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

const optional = z.string().optional().catch(undefined);

/** A24 search: `?env` the environment, `?from` and `?to` the range as dates (YYYY-MM-DD). */
export const analyticsSearchSchema = z.object({ env: optional, from: optional, to: optional });

/** A25 search: the range plus the drill-down, `?plugin` then `?screen`. */
export const usageSearchSchema = analyticsSearchSchema.extend({ plugin: optional, screen: optional });

export type AnalyticsSearch = z.infer<typeof analyticsSearchSchema>;
