// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { MessageInitShape } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { ExperimentService, type ExperimentState, FlagService, type FlagSpecSchema } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";

export function useExperiments(app: string, environmentId: string | undefined, state: ExperimentState | 0) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["experiments", app, environmentId, state],
    enabled: environmentId !== undefined,
    queryFn: () =>
      createClient(ExperimentService, session.transport).listExperiments({
        appId: app,
        environmentId: environmentId ?? "",
        state,
      }),
  });
}

export function useExperiment(id: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["experiment", id],
    queryFn: () => createClient(ExperimentService, session.transport).getExperiment({ id }),
  });
}

export function useResults(id: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["experiment-results", id],
    queryFn: () => createClient(ExperimentService, session.transport).getResults({ id }),
  });
}

export function useFlags(app: string, environmentId: string | undefined) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["flags", app, environmentId],
    enabled: environmentId !== undefined,
    queryFn: () =>
      createClient(FlagService, session.transport).listFlags({ appId: app, environmentId: environmentId ?? "" }),
  });
}

/** The mutations of the area; each refreshes what it changed. */
export function useExperimentActions(app: string, environmentId: string) {
  const { session } = useApp();
  const experiments = createClient(ExperimentService, session.transport);
  const flags = createClient(FlagService, session.transport);
  const refresh = async () => {
    for (const key of ["experiments", "experiment", "experiment-results", "flags"]) {
      await session.queryClient.invalidateQueries({ queryKey: [key] });
    }
  };
  const done = <T>(call: Promise<T>) => call.then(refresh);
  return {
    create: (document: Uint8Array) => experiments.createExperiment({ appId: app, environmentId, document }),
    start: (id: string) => done(experiments.startExperiment({ id, layerSharePercent: 100 })),
    stop: (id: string) => done(experiments.stopExperiment({ id })),
    ship: (id: string, variant: string, label: string) =>
      experiments.promoteExperiment({ id, variant, environmentId, label }).then((r) => refresh().then(() => r)),
    createFlag: (key: string, spec: MessageInitShape<typeof FlagSpecSchema>) =>
      done(flags.createFlag({ appId: app, environmentId, key, spec })),
    updateFlag: (key: string, spec: MessageInitShape<typeof FlagSpecSchema>) =>
      done(flags.updateFlag({ appId: app, environmentId, key, spec })),
    deleteFlag: (key: string) => done(flags.deleteFlag({ appId: app, environmentId, key })),
  };
}
