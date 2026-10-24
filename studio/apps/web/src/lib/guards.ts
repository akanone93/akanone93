// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Code, ConnectError } from "@connectrpc/connect";
import { type GetCurrentUserResponse, IdentityService, LicenseService } from "@plux/api";
import { createClient } from "@connectrpc/connect";
import { notFound, redirect } from "@tanstack/react-router";
import { organizationIdOf, type Session } from "./session.ts";

/** Permission that marks an installation administrator (R2.4). */
export const installationAdminPermission = "installation.admin";

/** Thrown by a guard; the router's error component renders the in-shell 403 or licence page. */
export class AccessError extends Error {
  constructor(
    readonly kind: "forbidden" | "licence",
    readonly permission: string,
  ) {
    super(kind);
    this.name = "AccessError";
  }
}

const userKey = ["current-user"] as const;

/** Only same-origin paths may be a return target. */
export function safeNext(next: string | undefined): string {
  return next !== undefined && next.startsWith("/") && !next.startsWith("//") ? next : "/";
}

/** The signed-in user, or undefined when the BFF answers 401. */
export async function loadUser(session: Session): Promise<GetCurrentUserResponse | undefined> {
  const client = createClient(IdentityService, session.transport);
  try {
    return await session.queryClient.fetchQuery({ queryKey: userKey, queryFn: () => client.getCurrentUser({}) });
  } catch (error) {
    if (error instanceof ConnectError && error.code === Code.Unauthenticated) {
      return undefined;
    }
    throw error;
  }
}

export function forgetUser(session: Session): void {
  session.queryClient.removeQueries({ queryKey: userKey });
}

/** Signed out: to the sign-in page with the path to return to (R2.7). */
export async function requireUser(session: Session, href: string): Promise<GetCurrentUserResponse> {
  const user = await loadUser(session);
  if (user === undefined) {
    throw redirect({ to: "/signin", search: { next: href } });
  }
  return user;
}

/** A member of the organisation, else 404 — never 403, so organisations stay undiscoverable (R1). */
export function requireMembership(user: GetCurrentUserResponse, orgParam: string): string {
  const id = organizationIdOf(orgParam);
  if (!user.memberships.some((m) => m.organizationId === id)) {
    throw notFound();
  }
  return id;
}

export function requirePermission(user: GetCurrentUserResponse, permission: string): void {
  if (!user.permissions.includes(permission)) {
    throw new AccessError("forbidden", permission);
  }
}

export function requireInstallationAdmin(user: GetCurrentUserResponse): void {
  requirePermission(user, installationAdminPermission);
}

/** Enterprise pages need a valid licence: administrators see the licence page, others a 404 (R1). */
export async function requireLicence(session: Session, user: GetCurrentUserResponse, adminPermission: string): Promise<void> {
  const state = await createClient(LicenseService, session.transport).getLicenseState({});
  if (state.state !== "absent" && state.state !== "invalid") {
    return;
  }
  if (user.permissions.includes(adminPermission)) {
    throw new AccessError("licence", adminPermission);
  }
  throw notFound();
}
