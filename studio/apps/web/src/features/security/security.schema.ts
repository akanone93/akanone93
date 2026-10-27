// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

/** A31 search: `?env&q`. Bad values fall back to the defaults. */
export const devicesSearchSchema = z.object({
  env: z.string().optional().catch(undefined),
  q: z.string().optional().catch(undefined),
});

/** A33-A36 search: `?env`. */
export const envSearchSchema = z.object({ env: z.string().optional().catch(undefined) });

export const defaultEnvironment = "development";
