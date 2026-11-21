// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { DescService } from "@bufbuild/protobuf";
import { type Client, createClient } from "@connectrpc/connect";
import { LicenseService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { useState } from "react";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { useCurrentUser } from "../../lib/use-current-user.ts";

/** The largest page the administration lists ask for; the server may return fewer. */
export const pageSize = 200;

/** The organisation of the path. */
export function useOrgId(): string {
  return organizationIdOf((useParams({ strict: false }) as { org?: string }).org ?? "");
}

/** The client of a service over the session's transport. */
export function useRpc<S extends DescService>(service: S): Client<S> {
  return createClient(service, useApp().session.transport);
}

/** Whether the signed-in person holds `permission`; actions without it are disabled, never hidden (F14). */
export function useCan(permission: string): boolean {
  const user = useCurrentUser();
  return user.data?.permissions.includes(permission) ?? false;
}

/**
 * Whether the installation's licence lets this organisation change an enterprise feature (GOV-030, GOV-033): false
 * when the licence is absent, invalid or expired, or does not list the feature; undefined while it is read, or when the
 * person may not read it, so that the server decides and says why.
 */
export function useLicensed(feature: string): boolean | undefined {
  const licence = useRpc(LicenseService);
  const state = useAdminQuery(["license"], () => licence.getLicenseState({}));
  if (state.data === undefined) {
    return undefined;
  }
  return (state.data.state === "valid" || state.data.state === "grace") && state.data.features.includes(feature);
}

/** A read of the administration area; every key starts with `admin` so one mutation refreshes them all. */
export function useAdminQuery<T>(key: readonly unknown[], read: () => Promise<T>, enabled = true) {
  return useQuery({ queryKey: ["admin", ...key], queryFn: read, enabled });
}

/**
 * Runs one change and refreshes the area's reads; `failed` tells the page to show the error and `error` is the
 * refusal itself, whose code and reason the page shows (a step-up is asked for by the session before it gets here).
 */
export function useRun() {
  const { session } = useApp();
  const [error, setError] = useState<unknown>(null);
  const run = async (change: () => Promise<unknown>): Promise<boolean> => {
    try {
      await change();
      setError(null);
      await session.queryClient.invalidateQueries({ queryKey: ["admin"] });
      return true;
    } catch (e) {
      setError(e);
      return false;
    }
  };
  return { run, failed: error !== null, error };
}

/** A comma-separated field as a list without blanks. */
export function splitList(text: string): string[] {
  return text
    .split(",")
    .map((part) => part.trim())
    .filter((part) => part !== "");
}
