// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

/** O7 search: `?app&status&mine`. Bad values fall back to the defaults. */
export const approvalsInboxSearchSchema = z.object({
  app: z.string().optional().catch(undefined),
  status: z.enum(["open", "approved", "rejected", "expired", "superseded", "used"]).optional().catch(undefined),
  mine: z.boolean().optional().catch(undefined),
});

/** O8-O12 search: `?app` names the app whose requests and policies are shown; absent means the organisation's own. */
export const appSearchSchema = z.object({ app: z.string().optional().catch(undefined) });

/** The statuses O9 shows: every request that is no longer waiting. */
export const decidedStatuses: readonly string[] = ["approved", "rejected", "expired", "superseded", "used"];

/** The kinds of artifact a policy can guard (policy schema). */
export const policyResources = [
  "release",
  "functionVersion",
  "policy",
  "securityConfiguration",
  "limit",
  "capabilitySet",
  "experiment",
  "waiver",
  "goldenBaseline",
] as const;

/** The risk attributes a policy can require (policy schema). */
export const policyRisks = ["touchesSecurePage", "touchesMoneyFlow", "changesFunctionUsedBy"] as const;
