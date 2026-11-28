// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampMs } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { type ApprovalRequest, ApprovalService, AppService, BreakGlassService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useParams, useSearch } from "@tanstack/react-router";
import { useApp } from "../../lib/use-app.tsx";
import { useCurrentUser } from "../../lib/use-current-user.ts";

/** The organisation of the path and the app of `?app` (empty: the organisation's own). */
export function useScope() {
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const app = (useSearch({ strict: false }) as { app?: string }).app ?? "";
  return { org, app };
}

/** The signed-in person: their id and permissions (F14). Actions the person lacks are disabled, never hidden. */
export function useMe() {
  const user = useCurrentUser();
  return { id: user.data?.user?.id ?? "", permissions: new Set(user.data?.permissions ?? []) };
}

export function useApps() {
  const { session } = useApp();
  return useQuery({
    queryKey: ["apps"],
    queryFn: () => createClient(AppService, session.transport).listApps({}),
  });
}

const createdMs = (r: ApprovalRequest) => (r.createdAt === undefined ? 0 : timestampMs(r.createdAt));

/**
 * The requests of `?app`, newest first. The server lists one scope per call, an empty app being the organisation's own
 * requests only, so "All apps" asks for the organisation's and for each app's the person can read, and merges them;
 * an app whose requests the person may not read is left out.
 */
export function useApprovals(app: string, status: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["approvals", app, status],
    queryFn: async (): Promise<{ approvals: ApprovalRequest[] }> => {
      const client = createClient(ApprovalService, session.transport);
      if (app !== "") {
        return { approvals: (await client.listApprovals({ appId: app, status })).approvals };
      }
      const apps = await createClient(AppService, session.transport).listApps({});
      const scopes = ["", ...apps.apps.map((a) => a.id)];
      const lists = await Promise.allSettled(scopes.map((appId) => client.listApprovals({ appId, status })));
      const approvals = lists.flatMap((l) => {
        if (l.status === "fulfilled") {
          return l.value.approvals;
        }
        if (l.reason instanceof ConnectError && l.reason.code === Code.PermissionDenied) {
          return [];
        }
        throw l.reason;
      });
      return { approvals: approvals.sort((a, b) => createdMs(b) - createdMs(a)) };
    },
  });
}

export function useApproval(app: string, id: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["approval", app, id],
    queryFn: () => createClient(ApprovalService, session.transport).getApprovalReview({ appId: app, id }),
  });
}

/** Sends the decision; the caller has completed the step-up first (D47, SEC-103). */
export function useDecide() {
  const { session } = useApp();
  return async (app: string, id: string, decision: "approve" | "reject", comment: string) => {
    await createClient(ApprovalService, session.transport).decideApproval({ appId: app, id, decision, comment });
    await session.queryClient.invalidateQueries({ queryKey: ["approval", app, id] });
    await session.queryClient.invalidateQueries({ queryKey: ["approvals"] });
  };
}

export function useBreakGlass() {
  const { session } = useApp();
  return useQuery({
    queryKey: ["break-glass"],
    queryFn: () => createClient(BreakGlassService, session.transport).listBreakGlass({}),
  });
}

export function useReviewBreakGlass() {
  const { session } = useApp();
  return async (id: string, comment: string) => {
    await createClient(BreakGlassService, session.transport).reviewBreakGlass({ id, comment });
    await session.queryClient.invalidateQueries({ queryKey: ["break-glass"] });
  };
}

export function usePolicies(app: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["policies", app],
    queryFn: () => createClient(ApprovalService, session.transport).listPolicies({ appId: app }),
  });
}

export function usePutPolicy() {
  const { session } = useApp();
  return async (app: string, document: Uint8Array) => {
    await createClient(ApprovalService, session.transport).putPolicy({ appId: app, document });
    await session.queryClient.invalidateQueries({ queryKey: ["policies", app] });
  };
}

export function useSimulate() {
  const { session } = useApp();
  return (
    app: string,
    document: Uint8Array,
    change: { resource: string; environment: string; action: string; risk: string[] },
  ) => createClient(ApprovalService, session.transport).simulatePolicy({ appId: app, document, change });
}

/** What the server said, for a refusal the person has to read (for example the author deciding their own request). */
export function refusalOf(error: unknown): string {
  return ConnectError.from(error).rawMessage;
}
