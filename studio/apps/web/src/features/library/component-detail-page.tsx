// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService } from "@plux/api";
import { Button } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { AdoptDialog } from "./adopt-dialog.tsx";
import { libraryMessages } from "./library.messages.ts";
import { saveFile } from "./library-model.ts";
import { useCan, useLibraryClients } from "./use-library.ts";
import { VisibilityDialog } from "./visibility-dialog.tsx";

/** O5: one component's versions, its instances across apps and the version each app adopted (STU-084, STU-085). */
export function ComponentDetailPage(): ReactElement {
  const m = libraryMessages(useLingui().i18n);
  const { session } = useApp();
  const { org = "", id = "" } = useParams({ strict: false }) as { org?: string; id?: string };
  const clients = useLibraryClients();
  const canPublish = useCan("library.publish");
  const canManage = useCan("library.manage");
  const canEdit = useCan("plugin.edit");
  const [dialog, setDialog] = useState<"visibility" | string | undefined>();
  const detail = useQuery({
    queryKey: ["library", "component", id],
    queryFn: () => clients.components.listComponentVersions({ id }),
  });
  const item = detail.data?.item;
  const perApp = useQuery({
    queryKey: ["library", "component-usage", id, item?.documentId],
    enabled: item !== undefined,
    queryFn: async () => {
      const apps = (
        await createClient(AppService, session.transport).listApps({ organizationId: organizationIdOf(org) })
      ).apps;
      return Promise.all(
        apps.map(async (app) => {
          const [usages, adoptions] = await Promise.all([
            clients.components.listUsages({ appId: app.id, entityKind: "component", entityId: item?.documentId ?? id }),
            clients.components.listComponentAdoptions({ appId: app.id }),
          ]);
          return { app, count: usages.usages.length, adopted: adoptions.adoptions.find((a) => a.itemId === id) };
        }),
      );
    },
  });
  const versions = [...(detail.data?.versions ?? [])].sort((a, b) => b.version - a.version);
  const exportVersion = async (version: number) => {
    const file = await clients.components.exportComponent({ id, version });
    saveFile(file.fileName, file.file);
  };
  const adoptTarget = perApp.data?.find((a) => a.app.id === dialog);
  return (
    <section className="flex flex-col gap-4" aria-labelledby="component-title">
      <Link to="/o/$org/library" params={{ org }} className="text-caption">
        {m.back}
      </Link>
      {detail.isPending && <p className="text-body text-secondary">{m.loading}</p>}
      {detail.isError && (
        <p role="alert" className="text-body text-danger">
          {m.detailLoadFailed}
        </p>
      )}
      {item !== undefined && (
        <>
          <h1 id="component-title" className="text-screen">
            {item.name}
          </h1>
          <p className="text-body text-secondary">{item.description}</p>
          {(canPublish || canManage) && (
            <div>
              <Button variant="secondary" onClick={() => setDialog("visibility")}>
                {m.changeVisibility}
              </Button>
            </div>
          )}
          <h2 className="text-body font-medium">{m.versionsHeading}</h2>
          <ul>
            {versions.map((v) => (
              <li key={v.version} className="flex items-center gap-3 py-1">
                <span className="text-body">{m.versionRow(v.version, v.createdByName)}</span>
                <span className="text-caption text-secondary">{v.note}</span>
                <Button variant="ghost" size="sm" onClick={() => void exportVersion(v.version)}>
                  {m.exportVersion}
                </Button>
              </li>
            ))}
          </ul>
          <h2 className="text-body font-medium">{m.usageHeading}</h2>
          {perApp.data?.every((a) => a.count === 0) === true && (
            <p className="text-body text-secondary">{m.usageNone}</p>
          )}
          <ul>
            {(perApp.data ?? []).map(({ app, count, adopted }) => (
              <li key={app.id} className="flex flex-wrap items-center gap-3 py-1">
                <span className="text-body">{m.usageApp(app.name, count)}</span>
                <span className="text-caption text-secondary">
                  {adopted === undefined ? m.notAdopted(app.name) : m.adoptedVersion(app.name, adopted.version)}
                </span>
                {canEdit && (
                  <Button variant="ghost" size="sm" onClick={() => setDialog(app.id)}>
                    {m.adoptVersion}
                  </Button>
                )}
              </li>
            ))}
          </ul>
        </>
      )}
      {dialog === "visibility" && item !== undefined && (
        <VisibilityDialog
          current={item.visibility}
          canPublish={canPublish}
          apply={async (visibility) => {
            await clients.components.setComponentVisibility({ id, visibility });
            await clients.invalidate();
          }}
          onClose={() => setDialog(undefined)}
        />
      )}
      {adoptTarget !== undefined && (
        <AdoptDialog
          appId={adoptTarget.app.id}
          appName={adoptTarget.app.name}
          versions={versions}
          adopted={adoptTarget.adopted}
          adopt={async (appId, version) => {
            const res = await clients.components.adoptComponentVersion({ appId, id, version });
            await clients.invalidate();
            return res.conflicts;
          }}
          onClose={() => setDialog(undefined)}
        />
      )}
    </section>
  );
}
