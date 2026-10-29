// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { DescService } from "@bufbuild/protobuf";
import { type Client, createClient } from "@connectrpc/connect";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { useState } from "react";
import { loadUser } from "../../lib/guards.ts";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";

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
  const { session } = useApp();
  const user = useQuery({ queryKey: ["current-user"], queryFn: () => loadUser(session) });
  return user.data?.permissions.includes(permission) ?? false;
}

/** A read of the administration area; every key starts with `admin` so one mutation refreshes them all. */
export function useAdminQuery<T>(key: readonly unknown[], read: () => Promise<T>, enabled = true) {
  return useQuery({ queryKey: ["admin", ...key], queryFn: read, enabled });
}

/** Runs one change and refreshes the area's reads; `failed` tells the page to show the error. */
export function useRun() {
  const { session } = useApp();
  const [failed, setFailed] = useState(false);
  const run = async (change: () => Promise<unknown>): Promise<boolean> => {
    try {
      await change();
      setFailed(false);
      await session.queryClient.invalidateQueries({ queryKey: ["admin"] });
      return true;
    } catch {
      setFailed(true);
      return false;
    }
  };
  return { run, failed };
}

/** A comma-separated field as a list without blanks. */
export function splitList(text: string): string[] {
  return text
    .split(",")
    .map((part) => part.trim())
    .filter((part) => part !== "");
}
