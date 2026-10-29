// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { MessageInitShape } from "@bufbuild/protobuf";
import { ConnectError, createClient } from "@connectrpc/connect";
import {
  type AdvanceReleaseRequestRequestSchema,
  PluginService,
  PublishService,
  type ReleaseRequest,
  ReleaseRequestService,
  type SelectedPlugin,
  type SelectedPluginSchema,
} from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { loadUser } from "../../lib/guards.ts";
import { useApp } from "../../lib/use-app.tsx";

type StepPayload = NonNullable<MessageInitShape<typeof AdvanceReleaseRequestRequestSchema>["step"]>;

const terminalJob = new Set(["succeeded", "failed", "cancelled"]);

/** The plugins of the app with their ids, keys and whether the draft changed (STU-100 step 1). */
export function useSummaries(app: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["plugin-summaries", app],
    queryFn: () => createClient(PluginService, session.transport).listPluginSummaries({ appId: app }),
  });
}

/** The release request as the server holds it; polls while a build, a diff or an approval is in flight. */
export function useReleaseRequest(app: string, id: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["release-request", app, id],
    queryFn: () => createClient(ReleaseRequestService, session.transport).getReleaseRequest({ appId: app, id }),
    refetchInterval: (query) => {
      const data = query.state.data;
      const diff = data?.visualDiff?.state;
      const waiting = data?.request?.hold === "awaiting_approval" || diff === "queued" || diff === "running";
      return waiting ? 2000 : false;
    },
  });
}

/** The state of the publish jobs the build step started; polls until each is finished. */
export function useJobs(jobIds: readonly string[]) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["publish-jobs", ...jobIds],
    enabled: jobIds.length > 0,
    queryFn: async () => {
      const client = createClient(PublishService, session.transport);
      return Promise.all(jobIds.map(async (jobId) => (await client.getPublishJob({ jobId })).job));
    },
    refetchInterval: (query) => (query.state.data?.every((j) => j && terminalJob.has(j.state)) ? false : 1500),
  });
}

/** The RPCs of one release request, each bound to it; the idempotency key of a build is stable per request revision (SRV-005). */
export function useReleaseApi(
  app: string,
  request: Pick<ReleaseRequest, "id" | "state" | "revision" | "environmentId">,
) {
  const { session } = useApp();
  const client = createClient(ReleaseRequestService, session.transport);
  const ids = { appId: app, id: request.id };
  return {
    advance: (step?: StepPayload) =>
      client.advanceReleaseRequest({ ...ids, expectedState: request.state, ...(step !== undefined && { step }) }),
    refreeze: () => client.refreezeReleaseRequest(ids),
    requestDiff: () => client.requestVisualDiff(ids),
    requestApproval: () => client.requestReleaseApproval(ids),
    complete: () => client.completeReleaseRequest(ids),
    cancel: () => client.cancelReleaseRequest(ids),
    /** Starts, or re-attaches to, one publish job per plugin; the same key returns the original job. */
    startBuilds: async (plugins: readonly SelectedPlugin[], pluginIds: ReadonlyMap<string, string>) => {
      const publish = createClient(PublishService, session.transport);
      return Promise.all(
        plugins.map(async (p) => {
          const { job } = await publish.publish(
            { appId: app, pluginId: pluginIds.get(p.pluginKey) ?? "", environmentId: request.environmentId },
            { headers: { "Idempotency-Key": `${request.id}:${p.pluginKey}:${request.revision}` } },
          );
          return { pluginKey: p.pluginKey, jobId: job?.id ?? "" };
        }),
      );
    },
  };
}

export type ReleaseApi = ReturnType<typeof useReleaseApi>;

/** A1: resumes the open release request of the app or opens one with the changed plugins (one open request per app and channel). */
export function useStartRequest() {
  const { session } = useApp();
  return async (
    appId: string,
    environmentId: string,
    plugins: MessageInitShape<typeof SelectedPluginSchema>[],
  ): Promise<string | undefined> => {
    const client = createClient(ReleaseRequestService, session.transport);
    if (plugins.length === 0) {
      const { requests } = await client.listReleaseRequests({ appId, openOnly: true });
      return requests[0]?.id;
    }
    return (await client.createReleaseRequest({ appId, environmentId, plugins })).request?.id;
  };
}

/** The id of the signed-in person, to tell their own request from someone else's. */
export function useUserId(): string {
  const { session } = useApp();
  const user = useQuery({ queryKey: ["current-user"], queryFn: () => loadUser(session) });
  return user.data?.user?.id ?? "";
}

/** The stable PLX code of a failed call (PLX-8060 to PLX-8062), or empty. */
export function codeOf(err: unknown): string {
  if (!(err instanceof ConnectError)) {
    return "";
  }
  return /PLX-806[0-2]/.exec(err.rawMessage)?.[0] ?? "";
}

/**
 * One rendered image of the request's visual diff as an object URL, fetched by its SHA-256 with the caller's read
 * access. The hash names the content, so the bytes are kept for the session; the URL is revoked when the tile goes.
 */
export function useDiffImage(appId: string, requestId: string, sha256: string) {
  const { session } = useApp();
  const png = useQuery({
    queryKey: ["visual-diff-image", appId, requestId, sha256],
    enabled: sha256 !== "",
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
    queryFn: async () =>
      (
        await createClient(ReleaseRequestService, session.transport).getVisualDiffImage({
          appId,
          id: requestId,
          sha256,
        })
      ).png,
  });
  const [url, setUrl] = useState<string | undefined>(undefined);
  useEffect(() => {
    if (png.data === undefined) {
      setUrl(undefined);
      return;
    }
    const made = URL.createObjectURL(new Blob([png.data as BlobPart], { type: "image/png" }));
    setUrl(made);
    return () => URL.revokeObjectURL(made);
  }, [png.data]);
  return { url, failed: png.isError };
}
