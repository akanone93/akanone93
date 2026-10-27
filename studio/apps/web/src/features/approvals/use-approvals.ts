// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { ConnectError, createClient } from "@connectrpc/connect";
import { ApprovalService, AppService, BreakGlassService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useParams, useSearch } from "@tanstack/react-router";
import { loadUser } from "../../lib/guards.ts";
import { useApp } from "../../lib/use-app.tsx";

/** The organisation of the path and the app of `?app` (empty: the organisation's own). */
export function useScope() {
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const app = (useSearch({ strict: false }) as { app?: string }).app ?? "";
  return { org, app };
}

/** The signed-in person: their id and permissions (F14). Actions the person lacks are disabled, never hidden. */
export function useMe() {
  const { session } = useApp();
  const user = useQuery({ queryKey: ["current-user"], queryFn: () => loadUser(session) });
  return { id: user.data?.user?.id ?? "", permissions: new Set(user.data?.permissions ?? []) };
}

export function useApps() {
  const { session } = useApp();
  return useQuery({
    queryKey: ["apps"],
    queryFn: () => createClient(AppService, session.transport).listApps({}),
  });
}

export function useApprovals(app: string, status: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["approvals", app, status],
    queryFn: () => createClient(ApprovalService, session.transport).listApprovals({ appId: app, status }),
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
