// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService, OrgService } from "@plux/api";
import { Button } from "@plux/ui";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { useState } from "react";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { useCan } from "../admin/use-admin.ts";
import { OnboardingChecklist } from "../onboarding/index.ts";
import { CreateAppDialog } from "./create-app-dialog.tsx";
import { homeMessages } from "./home.messages.ts";

const APPS_PAGE = 100;

/** O1: the organisation home with its apps. The other cards arrive with their slices (STU-015). */
export function HomePage() {
  const m = homeMessages(useLingui().i18n);
  const { session } = useApp();
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const id = organizationIdOf(org);
  const canCreate = useCan("app.manage");
  const [creating, setCreating] = useState(false);
  const orgs = useQuery({
    queryKey: ["organisations"],
    queryFn: () => createClient(OrgService, session.transport).listOrganizations({}),
  });
  const apps = useInfiniteQuery({
    queryKey: ["apps", id],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      createClient(AppService, session.transport).listApps({
        organizationId: id,
        page: { pageSize: APPS_PAGE, pageToken: pageParam },
      }),
    getNextPageParam: (last) => (last.page?.nextPageToken ? last.page.nextPageToken : undefined),
  });
  const name = orgs.data?.organizations.find((o) => o.id === id)?.name ?? "";
  const list = apps.data?.pages.flatMap((p) => p.apps) ?? [];
  return (
    <section aria-labelledby="home-title" className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <h1 id="home-title" className="text-screen">
          {m.title(name)}
        </h1>
        <p className="text-body text-secondary">{m.caption}</p>
      </div>
      <OnboardingChecklist org={org} canCreate={canCreate} onCreate={() => setCreating(true)} />
      <section aria-labelledby="home-apps" className="flex flex-col gap-3">
        <div className="flex items-center gap-4">
          <h2 id="home-apps" className="text-section">
            {m.appsTitle}
          </h2>
          {canCreate && list.length > 0 && <Button onClick={() => setCreating(true)}>{m.createApp}</Button>}
        </div>
        {apps.isError && (
          <p role="alert" className="text-body text-danger">
            {m.appsLoadFailed}
          </p>
        )}
        {apps.isSuccess && list.length === 0 && (
          <div className="flex flex-col items-start gap-2">
            <p className="text-body text-secondary">{m.appsEmpty}</p>
            {!canCreate && <p className="text-body text-secondary">{m.appsEmptyAsk}</p>}
            {canCreate && <Button onClick={() => setCreating(true)}>{m.createApp}</Button>}
          </div>
        )}
        {list.length > 0 && (
          <ul className="flex flex-col gap-1">
            {list.map((app) => (
              <li key={app.id}>
                <Link
                  to="/o/$org/apps/$app"
                  params={{ org, app: app.id }}
                  aria-label={m.openApp(app.name)}
                  className="text-body"
                >
                  {app.name} <span className="text-data text-secondary">{app.key}</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
        {apps.hasNextPage && (
          <div>
            <Button variant="secondary" loading={apps.isFetchingNextPage} onClick={() => void apps.fetchNextPage()}>
              {m.appsMore}
            </Button>
          </div>
        )}
      </section>
      {canCreate && <CreateAppDialog open={creating} onOpenChange={setCreating} organizationId={id} org={org} />}
    </section>
  );
}
