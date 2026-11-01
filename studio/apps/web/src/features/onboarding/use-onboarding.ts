// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { useMutation, useQuery } from "@connectrpc/connect-query";
import { PreferencesSchema, PreferencesService } from "@plux/api";

/** The steps in the order the specification lists them (STU-015); the server may add more, which follow. */
const order = ["create_app", "pair_device", "edit_screen", "publish_release", "invite_member"] as const;

function rank(key: string): number {
  const index = (order as readonly string[]).indexOf(key);
  return index === -1 ? order.length : index;
}

export interface Onboarding {
  /** The steps and the person's choice have loaded. */
  readonly ready: boolean;
  readonly steps: readonly { readonly key: string; readonly done: boolean }[];
  readonly done: number;
  /** Every step is done: the checklist is no longer offered. */
  readonly complete: boolean;
  /** The person put the checklist away; it stays reachable from the help menu. */
  readonly dismissed: boolean;
  /** Stores the choice in the person's preferences; false when the server refused it. */
  readonly setDismissed: (value: boolean) => Promise<boolean>;
}

/**
 * O1's state (STU-015), on the pages of an organisation (`enabled`): the organisation's steps, whose progress the server derives from what the organisation has done,
 * and whether the person dismissed the checklist.
 */
export function useOnboarding(enabled = true): Onboarding {
  const prefs = useQuery(PreferencesService.method.getPreferences, {});
  // The steps are the organisation's: outside one there are none to ask for.
  const onboarding = useQuery(PreferencesService.method.getOnboarding, {}, { enabled });
  const update = useMutation(PreferencesService.method.updatePreferences);
  const steps = [...(onboarding.data?.steps ?? [])].sort((a, b) => rank(a.key) - rank(b.key));
  const done = steps.filter((s) => s.done).length;
  const ready = steps.length > 0 && prefs.data !== undefined;
  return {
    ready,
    steps,
    done,
    complete: ready && done === steps.length,
    dismissed: prefs.data?.preferences?.onboardingDismissed === true,
    setDismissed: async (value) => {
      try {
        await update.mutateAsync({
          preferences: create(PreferencesSchema, { onboardingDismissed: value }),
          updateMask: { paths: ["onboarding_dismissed"] },
        });
        await prefs.refetch();
        return true;
      } catch {
        return false;
      }
    },
  };
}
