// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { ControlService, KillSwitchScope, ReleaseService, RolloutService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";

export function useReleases(app: string, environmentId: string | undefined) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["releases", app, environmentId],
    enabled: environmentId !== undefined,
    queryFn: () =>
      createClient(ReleaseService, session.transport).listReleases({ appId: app, environmentId: environmentId ?? "" }),
  });
}

export function useRelease(app: string, sequence: bigint) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["release", app, String(sequence)],
    queryFn: () => createClient(ReleaseService, session.transport).getRelease({ appId: app, sequence }),
  });
}

export function useChangelog(app: string, sequence: bigint) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["changelog", app, String(sequence)],
    queryFn: () => createClient(ReleaseService, session.transport).getChangelog({ appId: app, sequence }),
  });
}

export function useRollouts(app: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["rollouts", app],
    queryFn: () => createClient(RolloutService, session.transport).listRollouts({ appId: app, onlyOpen: false }),
  });
}

export function useControl(app: string, environmentId: string | undefined, channel: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["control", app, environmentId, channel],
    enabled: environmentId !== undefined,
    queryFn: () =>
      createClient(ControlService, session.transport).getControl({
        appId: app,
        environmentId: environmentId ?? "",
        channelKey: channel,
      }),
  });
}

/** The mutations of the area; each refreshes what it changed. */
export function useReleaseActions(app: string) {
  const { session } = useApp();
  const rollouts = createClient(RolloutService, session.transport);
  const releases = createClient(ReleaseService, session.transport);
  const control = createClient(ControlService, session.transport);
  const refresh = async () => {
    for (const key of ["rollouts", "releases", "release", "control"]) {
      await session.queryClient.invalidateQueries({ queryKey: [key, app] });
    }
  };
  const done = <T>(call: Promise<T>) => call.then(refresh);
  return {
    pause: (id: string) => done(rollouts.pauseRollout({ id })),
    resume: (id: string) => done(rollouts.resumeRollout({ id })),
    advance: (id: string) => done(rollouts.advanceRollout({ id })),
    abort: (id: string) => done(rollouts.abortRollout({ id })),
    reschedule: (id: string, endAt: Date | undefined) =>
      done(
        rollouts.rescheduleRollout({
          id,
          clearEndAt: endAt === undefined,
          ...(endAt !== undefined && { endAt: { seconds: BigInt(Math.floor(endAt.getTime() / 1000)), nanos: 0 } }),
        }),
      ),
    rollback: (environmentId: string, channelKey: string, toSequence: bigint) =>
      done(releases.rollbackRelease({ appId: app, environmentId, channelKey, toSequence })),
    setKillSwitch: (environmentId: string, channelKey: string, target: string) =>
      done(
        control.setKillSwitch({
          appId: app,
          environmentId,
          channelKey,
          killSwitch: { scope: KillSwitchScope.PLUGIN, target },
        }),
      ),
    clearKillSwitch: (environmentId: string, channelKey: string, target: string) =>
      done(
        control.clearKillSwitch({
          appId: app,
          environmentId,
          channelKey,
          killSwitch: { scope: KillSwitchScope.PLUGIN, target },
        }),
      ),
  };
}
