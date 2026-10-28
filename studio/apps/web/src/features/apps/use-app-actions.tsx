// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type App, AppProjectService, AppService, MetadataUpdateSchema } from "@plux/api";
import { notify } from "@plux/ui";
import { useNavigate } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { CreateDialog } from "../../components/dialogs/index.ts";
import { useApp } from "../../lib/use-app.tsx";
import { download } from "../admin/download.ts";
import type { AppActionHandlers } from "./app-actions.tsx";
import { appsMessages } from "./apps.messages.ts";
import { usePinnedApps, useRefreshApps } from "./use-apps.ts";

/**
 * What the card actions do over the API (STU-023): pin, archive and unarchive, export a `.plux` archive and
 * duplicate. `dialog` is the duplicate dialog while one is open; the area renders it.
 */
export function useAppActions(org: string): {
  readonly pinned: ReadonlySet<string>;
  readonly handlers: AppActionHandlers;
  readonly dialog: ReactElement | undefined;
} {
  const m = appsMessages(useLingui().i18n);
  const { session } = useApp();
  const navigate = useNavigate();
  const refresh = useRefreshApps();
  const { pinned, toggle } = usePinnedApps();
  const [duplicating, setDuplicating] = useState<App | undefined>(undefined);
  const fail = () => notify("error", m.actionFailed);
  const handlers: AppActionHandlers = {
    onPin: (id) => {
      void toggle(id).then((ok) => !ok && notify("error", m.pinFailed));
    },
    onArchive: (app, archived) => {
      createClient(AppService, session.transport)
        .updateApp({
          id: app.id,
          name: app.name,
          defaultPluginKey: app.defaultPluginKey,
          metadata: create(MetadataUpdateSchema, { archived }),
        })
        .then(refresh)
        .catch(fail);
    },
    onDuplicate: setDuplicating,
    onExport: (app) => {
      createClient(AppProjectService, session.transport)
        .exportApp({ appId: app.id })
        .then((res) => download(res.filename, new Uint8Array(res.archive), "application/zip"))
        .catch(fail);
    },
  };
  const dialog =
    duplicating === undefined ? undefined : (
      <CreateDialog
        open
        onOpenChange={(open) => !open && setDuplicating(undefined)}
        title={m.duplicateTitle(duplicating.name)}
        submitLabel={m.duplicateSubmit}
        onCreate={async ({ key, name }) => {
          const res = await createClient(AppProjectService, session.transport).duplicateApp({
            appId: duplicating.id,
            key,
            name,
          });
          await refresh();
          setDuplicating(undefined);
          void navigate({ to: "/o/$org/apps/$app", params: { org, app: res.app?.id ?? "" } });
        }}
      />
    );
  return { pinned, handlers, dialog };
}
