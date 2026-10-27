// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

/** The steps of the release request, in order (F11); the state of the server walks through them. */
export const steps = [
  "select",
  "build",
  "reports",
  "diffs",
  "changelog",
  "target",
  "approval",
  "rollout",
  "release",
] as const;

export type Step = (typeof steps)[number];

/** A2 search: `?step` shows one step; it mirrors the server state by default. */
export const publishSearchSchema = z.object({
  step: z.enum(steps).optional().catch(undefined),
  env: z.string().optional().catch(undefined),
});

export const endedStates = ["released", "rejected", "expired", "cancelled"] as const;

export type EndedState = (typeof endedStates)[number];

/** The step a request state shows: an ended request shows the last step. */
export function stepOf(state: string): Step {
  return (steps as readonly string[]).includes(state) ? (state as Step) : "release";
}

export function isEnded(state: string): state is EndedState {
  return (endedStates as readonly string[]).includes(state);
}
