// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

/** O23 search `?user&app&env&permission`: the question "explain access" answers. Bad values fall back to unset. */
export const accessSearchSchema = z.object({
  user: z.string().optional().catch(undefined),
  permission: z.string().optional().catch(undefined),
  app: z.string().optional().catch(undefined),
  env: z.string().optional().catch(undefined),
});

export type AccessSearch = z.infer<typeof accessSearchSchema>;

/** O27 search `?from&to&actor&action&resource`: the audit filters, as the text the person typed. */
export const auditSearchSchema = z.object({
  from: z.string().optional().catch(undefined),
  to: z.string().optional().catch(undefined),
  actor: z.string().optional().catch(undefined),
  action: z.string().optional().catch(undefined),
  resource: z.string().optional().catch(undefined),
});

export type AuditSearch = z.infer<typeof auditSearchSchema>;
