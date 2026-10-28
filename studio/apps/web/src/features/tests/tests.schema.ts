// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

/** A45 search: `?env` the environment of the links, `?q` filters scenarios by name, page or file. */
export const testsSearchSchema = z.object({
  env: z.string().optional().catch(undefined),
  q: z.string().optional().catch(undefined),
});

/** A46 search: `?env` is carried back to the list. */
export const testRunSearchSchema = z.object({
  env: z.string().optional().catch(undefined),
});
