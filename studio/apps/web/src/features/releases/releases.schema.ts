// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

/** A3 search: `?env&channel`. Bad values fall back to the defaults. */
export const releasesSearchSchema = z.object({
  env: z.string().optional().catch(undefined),
  channel: z.string().optional().catch(undefined),
});

/** A4 and A5 search: `?env`. */
export const releaseEnvSearchSchema = z.object({ env: z.string().optional().catch(undefined) });

/** The channel the timeline and the controls act on when `?channel` is absent. */
export const defaultChannel = "production";

/** The health figures of a rollout's newest check (REL-053), as the server stores them. */
export const checkSchema = z.object({
  decided: z.boolean().optional(),
  regressions: z.array(z.object({ metric: z.string(), worseningBasisPoints: z.number() })).optional(),
  candidate: z.object({ Sessions: z.number(), Crashed: z.number(), Errors: z.number() }),
  previous: z.object({ Sessions: z.number(), Crashed: z.number(), Errors: z.number() }),
});
export type Check = z.infer<typeof checkSchema>;

/** Parses the check JSON of a rollout; a rollout without a check has none. */
export function parseCheck(bytes: Uint8Array): Check | undefined {
  if (bytes.length === 0) {
    return undefined;
  }
  try {
    const parsed = checkSchema.safeParse(JSON.parse(new TextDecoder().decode(bytes)));
    return parsed.success ? parsed.data : undefined;
  } catch {
    return undefined;
  }
}
