// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { AppService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouterState } from "@tanstack/react-router";
import { useEffect } from "react";
import { chooseCurrentApp, rememberApp, rememberedApp } from "../../lib/current-app.ts";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";

/** An app as the navigation shows it. */
export interface NavApp {
  readonly id: string;
  readonly name: string;
}

/** A page of `listApps`; the navigation reads every page. */
const PAGE = 100;

/** The apps of `organisationId` the server lists for this person, in its order: the only apps the navigation offers. */
export function useNavApps(organisationId: string | undefined) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["nav-apps", organisationId],
    enabled: organisationId !== undefined,
    staleTime: 60_000,
    queryFn: async (): Promise<NavApp[]> => {
      const client = createClient(AppService, session.transport);
      const apps: NavApp[] = [];
      let pageToken = "";
      for (;;) {
        const res = await client.listApps({
          organizationId: organisationId ?? "",
          page: { pageSize: PAGE, pageToken },
        });
        apps.push(...res.apps.map((a) => ({ id: a.id, name: a.name })));
        const next = res.page?.nextPageToken ?? "";
        if (next === "" || next === pageToken) {
          return apps;
        }
        pageToken = next;
      }
    },
  });
}

/** Remembers the app of the page's address as the organisation's last opened app; the canvases call it too. */
export function useRememberAppInPath(): void {
  const { org, app } = useParams({ strict: false }) as { org?: string; app?: string };
  useEffect(() => {
    if (org !== undefined && app !== undefined) {
      rememberApp(organizationIdOf(org), app);
    }
  }, [org, app]);
}

/**
 * The app the navigation is about on this page, by the one rule of `chooseCurrentApp`, with the apps it may offer.
 * `organisationId` is the organisation the sidebar shows.
 */
export function useCurrentApp(organisationId: string | undefined): {
  readonly app: string | undefined;
  readonly apps: readonly NavApp[];
} {
  const { app: inPath } = useParams({ strict: false }) as { app?: string };
  const inSearch = useRouterState({
    select: (s) => {
      const value = (s.location.search as { app?: unknown }).app;
      return typeof value === "string" ? value : undefined;
    },
  });
  useRememberAppInPath();
  const apps = useNavApps(organisationId).data ?? [];
  const app =
    organisationId === undefined
      ? undefined
      : chooseCurrentApp({
          inPath,
          inSearch,
          remembered: rememberedApp(organisationId),
          readable: apps.map((a) => a.id),
        });
  return { app, apps };
}
