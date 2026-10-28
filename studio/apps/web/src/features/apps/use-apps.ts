// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { FieldMaskSchema } from "@bufbuild/protobuf/wkt";
import { createClient } from "@connectrpc/connect";
import { useMutation, useQuery } from "@connectrpc/connect-query";
import { type App, AppProjectService, AppService, OrgService, PreferencesSchema, PreferencesService } from "@plux/api";
import { limitDefault } from "@plux/schema";
import { useQueryClient, useQuery as useTanstackQuery } from "@tanstack/react-query";
import { useCallback, useMemo } from "react";
import { useApp } from "../../lib/use-app.tsx";
import type { AppsSearch } from "./apps.schema.ts";
import { type AppEntry, filterApps, sortApps, tagsOf } from "./apps-model.ts";

/** The page size of the app lists that carry no summaries. */
const LIST_PAGE = 100;

/**
 * Every active app of the organisation with its summary, read in batches of `app.summariesPerCall` (STU-022): the
 * grid asks for the summaries of all cards in a few calls, never one call per card.
 */
export function useAppEntries(organizationId: string) {
  const { session } = useApp();
  return useTanstackQuery({
    queryKey: ["apps", "summaries", organizationId],
    queryFn: async (): Promise<AppEntry[]> => {
      const client = createClient(AppProjectService, session.transport);
      const limit = limitDefault("app.summariesPerCall");
      const entries: AppEntry[] = [];
      let afterKey = "";
      for (;;) {
        const res = await client.listAppSummaries({ organizationId, limit, afterKey });
        for (const summary of res.summaries) {
          if (summary.app !== undefined) {
            entries.push({ app: summary.app, summary });
          }
        }
        if (res.nextAfterKey === "" || res.nextAfterKey === afterKey) {
          return entries;
        }
        afterKey = res.nextAfterKey;
      }
    },
  });
}

/** All pages of `listApps` with the given status and "mine" switch. */
async function listAll(
  client: ReturnType<typeof createClient<typeof AppService>>,
  organizationId: string,
  status: "archived" | "all",
  mine: boolean,
): Promise<App[]> {
  const apps: App[] = [];
  let pageToken = "";
  for (;;) {
    const res = await client.listApps({ organizationId, status, mine, page: { pageSize: LIST_PAGE, pageToken } });
    apps.push(...res.apps);
    const next = res.page?.nextPageToken ?? "";
    if (next === "" || next === pageToken) {
      return apps;
    }
    pageToken = next;
  }
}

/** The archived apps, which have no summaries; read only while the status filter asks for them. */
export function useArchivedEntries(organizationId: string, enabled: boolean) {
  const { session } = useApp();
  return useTanstackQuery({
    queryKey: ["apps", "archived", organizationId],
    enabled,
    queryFn: async (): Promise<AppEntry[]> =>
      (await listAll(createClient(AppService, session.transport), organizationId, "archived", false)).map((app) => ({
        app,
        summary: undefined,
      })),
  });
}

/** The IDs of the apps the caller holds a grant on, directly or through a team; read only while "my apps" is on. */
export function useMineIds(organizationId: string, enabled: boolean) {
  const { session } = useApp();
  return useTanstackQuery({
    queryKey: ["apps", "mine", organizationId],
    enabled,
    queryFn: async (): Promise<ReadonlySet<string>> =>
      new Set(
        (await listAll(createClient(AppService, session.transport), organizationId, "all", true)).map((a) => a.id),
      ),
  });
}

/** The teams of the organisation by ID, for the team name on a card and the team filter. */
export function useTeamNames(organizationId: string) {
  const { session } = useApp();
  const teams = useTanstackQuery({
    queryKey: ["teams", organizationId],
    queryFn: async () =>
      (await createClient(OrgService, session.transport).listTeams({ organizationId, page: { pageSize: 200 } })).teams,
  });
  return useMemo(() => (teams.data ?? []).map((t) => ({ id: t.id, name: t.name })), [teams.data]);
}

/** The apps the person pinned (their favourites in the preferences), and a way to pin or unpin one. */
export function usePinnedApps(): {
  readonly pinned: ReadonlySet<string>;
  readonly toggle: (id: string) => Promise<boolean>;
} {
  const prefs = useQuery(PreferencesService.method.getPreferences, {});
  const update = useMutation(PreferencesService.method.updatePreferences);
  const stored = prefs.data?.preferences?.favouriteAppIds;
  const pinned = useMemo(() => new Set(stored ?? []), [stored]);
  const { mutateAsync } = update;
  const { refetch } = prefs;
  const toggle = useCallback(
    async (id: string): Promise<boolean> => {
      const next = pinned.has(id) ? [...pinned].filter((p) => p !== id) : [...pinned, id];
      try {
        await mutateAsync({
          preferences: create(PreferencesSchema, { favouriteAppIds: next }),
          updateMask: create(FieldMaskSchema, { paths: ["favourite_app_ids"] }),
        });
        await refetch();
        return true;
      } catch {
        return false;
      }
    },
    [pinned, mutateAsync, refetch],
  );
  return { pinned, toggle };
}

/** Refreshes every list of apps and the app itself after a change. */
export function useRefreshApps(): () => Promise<void> {
  const queryClient = useQueryClient();
  return useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: ["apps"] });
    await queryClient.invalidateQueries({ queryKey: ["app"] });
  }, [queryClient]);
}

/** What the area shows for the URL's search: the entries after the filters and the order, and what the filters offer. */
export function useVisibleApps(organizationId: string, search: AppsSearch) {
  const status = search.status ?? "active";
  const mine = search.mine === true;
  const active = useAppEntries(organizationId);
  const archived = useArchivedEntries(organizationId, status !== "active");
  const mineIds = useMineIds(organizationId, mine);
  const teams = useTeamNames(organizationId);
  const { pinned } = usePinnedApps();
  const all = useMemo<AppEntry[]>(
    () => [
      ...(status === "archived" ? [] : (active.data ?? [])),
      ...(status === "active" ? [] : (archived.data ?? [])),
    ],
    [status, active.data, archived.data],
  );
  const shown = useMemo(
    () =>
      sortApps(
        filterApps(all, {
          q: search.q ?? "",
          team: search.team ?? "",
          tag: search.tag ?? "",
          mine: mine ? (mineIds.data ?? new Set<string>()) : undefined,
        }),
        search.sort ?? "name",
        pinned,
      ),
    [all, search.q, search.team, search.tag, search.sort, mine, mineIds.data, pinned],
  );
  const tags = useMemo(() => tagsOf(all), [all]);
  const anyApps = (active.data?.length ?? 0) > 0 || (archived.data?.length ?? 0) > 0;
  return { active, archived, teams, shown, tags, anyApps };
}
