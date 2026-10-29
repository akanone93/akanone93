// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

/** A20 to A23 search: `?env` is the environment whose URLs are shown, `?plugin` the plugin whose documents are edited. Bad values fall back to the defaults. */
export const dataSearchSchema = z.object({
  env: z.string().optional().catch(undefined),
  plugin: z.string().optional().catch(undefined),
});
export type DataSearch = z.infer<typeof dataSearchSchema>;
