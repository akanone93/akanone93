// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button } from "@plux/ui";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { type ReactElement, useMemo } from "react";
import { useCan } from "../admin/use-admin.ts";
import { homeMessages } from "../home/home.messages.ts";
import { AppCard, AppRow } from "./app-card.tsx";
import { appsMessages } from "./apps.messages.ts";
import { type AppsSearch, appsSearchSchema } from "./apps.schema.ts";
import { AppsToolbar, Toggle } from "./apps-toolbar.tsx";
import { useAppActions } from "./use-app-actions.tsx";
import { useVisibleApps } from "./use-apps.ts";

const gridClass = "grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4";

/** The loading, failure and empty states above the apps. */
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
        <p role="status" className="text-body text-secondary">
          {m.loading}
        </p>
      )}
      {failed && (
        <p role="alert" className="text-body text-danger">
          {home.appsLoadFailed}
        </p>
      )}
      {archivedFailed && (
        <p role="alert" className="text-body text-danger">
          {m.archivedLoadFailed}
        </p>
      )}
      {empty && (
        <div className="flex flex-col items-start gap-2">
          <p className="text-body text-secondary">{home.appsEmpty}</p>
          {onCreate === undefined && <p className="text-body text-secondary">{home.appsEmptyAsk}</p>}
          {onCreate !== undefined && <Button onClick={onCreate}>{home.createApp}</Button>}
        </div>
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
  /** Opens the create dialog; undefined when the person may not create apps. */
  readonly onCreate: (() => void) | undefined;
}): ReactElement {
  const { i18n } = useLingui();
  const m = appsMessages(i18n);
  const home = homeMessages(i18n);
  const navigate = useNavigate();
  const search: AppsSearch = appsSearchSchema.parse(useSearch({ strict: false }));
  const view = search.view ?? "grid";
  const change = (patch: AppsSearch) =>
    void navigate({ to: ".", search: (prev) => ({ ...prev, ...patch }), replace: true });
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
    <section aria-labelledby="home-apps" className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-4">
        <h2 id="home-apps" className="text-section">
          {home.appsTitle}
        </h2>
        {onCreate !== undefined && anyApps && <Button onClick={onCreate}>{home.createApp}</Button>}
        <fieldset aria-label={m.viewLabel} className="ms-auto flex min-w-0 gap-1">
          <Toggle on={view === "grid"} onClick={() => change({ view: "grid" })}>
            {m.viewGrid}
          </Toggle>
          <Toggle on={view === "list"} onClick={() => change({ view: "list" })}>
            {m.viewList}
          </Toggle>
        </fieldset>
      </div>
      {anyApps && <AppsToolbar search={search} teams={teams} tags={tags} change={change} />}
      <States
        loading={active.isPending}
        failed={active.isError}
        archivedFailed={archived.isError}
        empty={active.isSuccess && !anyApps && (search.status ?? "active") === "active"}
        onCreate={onCreate}
      />
      {anyApps && (
        <p role="status" className="text-caption text-secondary">
          {m.count(shown.length)}
        </p>
      )}
      {anyApps && shown.length === 0 && (
        <div className="flex flex-col items-start gap-2">
          <p className="text-body text-secondary">{m.noMatch}</p>
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
        </div>
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
