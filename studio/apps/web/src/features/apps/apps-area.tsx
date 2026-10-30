// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Icon } from "@plux/ui";
import { Alert, AlertTitle } from "@plux/ui/components/alert";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@plux/ui/components/empty";
import { Skeleton } from "@plux/ui/components/skeleton";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { type ReactElement, useMemo } from "react";
import { useCan } from "../admin/use-admin.ts";
import { homeMessages } from "../home/home.messages.ts";
import { AppCard, AppRow } from "./app-card.tsx";
import { appsMessages } from "./apps.messages.ts";
import { type AppsSearch, appsSearchSchema } from "./apps.schema.ts";
import { AppsToolbar, Choice } from "./apps-toolbar.tsx";
import { useAppActions } from "./use-app-actions.tsx";
import { useVisibleApps } from "./use-apps.ts";
import "./apps.css";

const gridClass = "grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3";

/** The loading, failure and empty states above the apps: shadcn's Skeleton, Alert and Empty. */
function States({
  loading,
  failed,
  archivedFailed,
  empty,
  onCreate,
}: {
  readonly loading: boolean;
  readonly failed: boolean;
  readonly archivedFailed: boolean;
  readonly empty: boolean;
  readonly onCreate: (() => void) | undefined;
}): ReactElement {
  const { i18n } = useLingui();
  const m = appsMessages(i18n);
  const home = homeMessages(i18n);
  return (
    <>
      {loading && (
        <div className={gridClass}>
          <p role="status" className="sr-only">
            {m.loading}
          </p>
          {[0, 1, 2].map((k) => (
            <Skeleton key={k} className="h-72 rounded-xl" />
          ))}
        </div>
      )}
      {failed && (
        <Alert variant="destructive">
          <Icon name="warning-circle" />
          <AlertTitle>{home.appsLoadFailed}</AlertTitle>
        </Alert>
      )}
      {archivedFailed && (
        <Alert variant="destructive">
          <Icon name="warning-circle" />
          <AlertTitle>{m.archivedLoadFailed}</AlertTitle>
        </Alert>
      )}
      {empty && (
        <Empty className="border border-dashed">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Icon name="squares-four" />
            </EmptyMedia>
            <EmptyTitle>{home.appsEmpty}</EmptyTitle>
            {onCreate === undefined && <EmptyDescription>{home.appsEmptyAsk}</EmptyDescription>}
          </EmptyHeader>
          {onCreate !== undefined && (
            <EmptyContent>
              <Button onClick={onCreate}>
                <Icon name="plus" data-icon="inline-start" />
                {home.createApp}
              </Button>
            </EmptyContent>
          )}
        </Empty>
      )}
    </>
  );
}

/**
 * The apps of the organisation (STU-020 to STU-025): a responsive grid of cards with a list alternative, search,
 * filters, sort, pins and the card actions. The summaries come in batches of `app.summariesPerCall`; the search,
 * filters and order are part of the URL (O2), so a view can be bookmarked and shared.
 */
export function AppsArea({
  org,
  organizationId,
  onCreate,
}: {
  /** The `:org` URL parameter, which the app paths continue. */
  readonly org: string;
  /** The organisation's immutable ID, which the requests carry. */
  readonly organizationId: string;
  /** Opens the create dialog from the empty state; undefined when the person may not create apps. */
  readonly onCreate: (() => void) | undefined;
}): ReactElement {
  const { i18n } = useLingui();
  const m = appsMessages(i18n);
  const home = homeMessages(i18n);
  const navigate = useNavigate();
  const search: AppsSearch = appsSearchSchema.parse(useSearch({ strict: false }));
  const view = search.view ?? "grid";
  const change = (patch: AppsSearch) =>
    // The page stays where it is: a filter changes the list below, not the place the person reads (O2).
    void navigate({ to: ".", search: (prev) => ({ ...prev, ...patch }), replace: true, resetScroll: false });
  const canManage = useCan("app.manage");
  const canReadPlugins = useCan("plugin.read");
  const permissions = { manage: canManage, readPlugins: canReadPlugins };
  const { active, archived, teams, shown, tags, anyApps } = useVisibleApps(organizationId, search);
  const { pinned, handlers, dialog } = useAppActions(org);
  const teamNames = useMemo(() => new Map(teams.map((t) => [t.id, t.name])), [teams]);
  const Item = view === "grid" ? AppCard : AppRow;
  const filtered =
    (search.q ?? "") !== "" ||
    search.team !== undefined ||
    search.tag !== undefined ||
    search.mine === true ||
    (search.status ?? "active") !== "active";
  return (
    <section aria-labelledby="home-apps" className="flex flex-col gap-4 px-4 lg:px-6">
      <div className="flex flex-wrap items-center gap-3">
        <h2 id="home-apps" className="text-lg font-semibold tracking-tight">
          {home.appsTitle}
        </h2>
        {anyApps && (
          <p role="status" className="text-sm text-muted-foreground">
            {m.count(shown.length)}
          </p>
        )}
        <div className="ms-auto">
          <Choice
            label={m.viewLabel}
            value={view}
            options={[
              { value: "grid", label: m.viewGrid, icon: "squares-four" },
              { value: "list", label: m.viewList, icon: "list" },
            ]}
            onChange={(v) => change({ view: v })}
          />
        </div>
      </div>
      {anyApps && <AppsToolbar search={search} teams={teams} tags={tags} change={change} />}
      <States
        loading={active.isPending}
        failed={active.isError}
        archivedFailed={archived.isError}
        empty={active.isSuccess && !anyApps && (search.status ?? "active") === "active"}
        onCreate={onCreate}
      />
      {anyApps && shown.length === 0 && (
        <Empty className="border border-dashed">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Icon name="magnifying-glass" />
            </EmptyMedia>
            <EmptyTitle>{m.noMatch}</EmptyTitle>
          </EmptyHeader>
          {filtered && (
            <Button
              variant="secondary"
              onClick={() =>
                change({ q: undefined, team: undefined, tag: undefined, status: undefined, mine: undefined })
              }
            >
              {m.clearFilters}
            </Button>
          )}
        </Empty>
      )}
      {shown.length > 0 && (
        <ul className={view === "grid" ? gridClass : "flex flex-col gap-2"}>
          {shown.map((entry) => (
            <Item
              key={entry.app.id}
              entry={entry}
              org={org}
              team={teamNames.get(entry.app.owningTeamId)}
              pinned={pinned.has(entry.app.id)}
              permissions={permissions}
              handlers={handlers}
            />
          ))}
        </ul>
      )}
      {dialog}
    </section>
  );
}
