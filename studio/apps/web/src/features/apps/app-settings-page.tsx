// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type App, AppService, MetadataUpdateSchema } from "@plux/api";
import { Button, Field, Input, Textarea } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { type FormEvent, type ReactElement, useState } from "react";
import { isName } from "../../lib/keys.ts";
import { useApp } from "../../lib/use-app.tsx";
import { useCan } from "../admin/use-admin.ts";
import { ShellSettingsLink } from "../shell-settings/index.ts";
import { appsMessages } from "./apps.messages.ts";
import { LimitsTable } from "./limits-usage.tsx";
import { PluginUsage } from "./plugin-usage.tsx";
import { useRefreshApps } from "./use-apps.ts";

type SaveState = "idle" | "saved" | "failed";

/** The name and description of the app (UpdateApp, needs app.manage); everyone who can read the app sees them. */
function DetailsForm({ app, canEdit }: { readonly app: App; readonly canEdit: boolean }): ReactElement {
  const m = appsMessages(useLingui().i18n);
  const { session } = useApp();
  const refresh = useRefreshApps();
  const [name, setName] = useState(app.name);
  const [description, setDescription] = useState(app.description);
  const [attempted, setAttempted] = useState(false);
  const [working, setWorking] = useState(false);
  const [state, setState] = useState<SaveState>("idle");
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setAttempted(true);
    if (working || !canEdit || !isName(name)) {
      return;
    }
    setWorking(true);
    try {
      await createClient(AppService, session.transport).updateApp({
        id: app.id,
        name: name.trim(),
        defaultPluginKey: app.defaultPluginKey,
        metadata: create(MetadataUpdateSchema, { description }),
      });
      setState("saved");
      await refresh();
    } catch {
      setState("failed");
    } finally {
      setWorking(false);
    }
  };
  return (
    <section aria-labelledby="app-settings-details" className="flex max-w-prose flex-col gap-3">
      <h2 id="app-settings-details" className="text-section">
        {m.detailsTitle}
      </h2>
      <form noValidate onSubmit={(e) => void submit(e)} className="flex flex-col gap-3">
        <Field label={m.nameLabel} error={attempted && !isName(name) ? m.nameInvalid : undefined}>
          <Input value={name} disabled={!canEdit} autoComplete="off" onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label={m.descriptionLabel}>
          <Textarea value={description} disabled={!canEdit} rows={3} onChange={(e) => setDescription(e.target.value)} />
        </Field>
        {!canEdit && <p className="text-body text-secondary">{m.readOnly}</p>}
        {canEdit && (
          <div className="flex items-center gap-3">
            <Button type="submit" loading={working}>
              {m.save}
            </Button>
            {state === "saved" && (
              <p role="status" className="text-body text-secondary">
                {m.saved}
              </p>
            )}
          </div>
        )}
        {state === "failed" && (
          <p role="alert" className="text-body text-danger">
            {m.saveFailed}
          </p>
        )}
      </form>
    </section>
  );
}

/**
 * The settings of one app (STU-025, LIM-005): its name and description, its usage against every limit, and the links
 * to the host fonts and to the organisation's administration, where the app's access and limits are set.
 */
export function AppSettingsPage(): ReactElement {
  const m = appsMessages(useLingui().i18n);
  const { session } = useApp();
  const { org, app: appId } = useParams({ strict: false }) as { org: string; app: string };
  const canEdit = useCan("app.manage");
  const canAdmin = useCan("organization.manage");
  const canPlugins = useCan("plugin.read");
  const app = useQuery({
    queryKey: ["app", appId],
    queryFn: async () => (await createClient(AppService, session.transport).getApp({ id: appId })).app,
  });
  const limits = useQuery({
    queryKey: ["app-limits", appId],
    queryFn: async () => (await createClient(AppService, session.transport).listAppLimits({ appId })).limits,
  });
  const loaded = app.data;
  return (
    <section aria-labelledby="app-settings-title" className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <h1 id="app-settings-title" className="text-screen">
          {m.settingsTitle(loaded?.name ?? "")}
        </h1>
        <p className="text-body text-secondary">{m.settingsCaption}</p>
        {loaded !== undefined && <p className="font-mono text-data text-secondary">{m.keyOf(loaded.key)}</p>}
      </div>
      {app.isPending && (
        <p role="status" className="text-body text-secondary">
          {m.loading}
        </p>
      )}
      {app.isError && (
        <p role="alert" className="text-body text-danger">
          {m.settingsLoadFailed}
        </p>
      )}
      {loaded !== undefined && <DetailsForm key={loaded.id} app={loaded} canEdit={canEdit} />}
      <section aria-labelledby="app-settings-limits" className="flex flex-col gap-3">
        <h2 id="app-settings-limits" className="text-section">
          {m.limitsTitle}
        </h2>
        <p className="text-body text-secondary">{m.limitsCaption}</p>
        {limits.isError && (
          <p role="alert" className="text-body text-danger">
            {m.limitsLoadFailed}
          </p>
        )}
        {limits.isSuccess && <LimitsTable limits={limits.data} />}
      </section>
      <section aria-labelledby="app-settings-shell" className="flex flex-col gap-2">
        <h2 id="app-settings-shell" className="text-section">
          {m.shellTitle}
        </h2>
        <p className="text-body text-secondary">{m.shellCaption}</p>
        <ShellSettingsLink org={org} appId={appId} />
      </section>
      {canPlugins && <PluginUsage appId={appId} />}
      <section aria-labelledby="app-settings-fonts" className="flex flex-col gap-2">
        <h2 id="app-settings-fonts" className="text-section">
          {m.fontsTitle}
        </h2>
        <p className="text-body text-secondary">{m.fontsCaption}</p>
        <Link to="/o/$org/apps/$app/design/typography" params={{ org, app: appId }} className="text-body">
          {m.fontsLink}
        </Link>
      </section>
      {canAdmin && (
        <section aria-labelledby="app-settings-admin" className="flex flex-col gap-2">
          <h2 id="app-settings-admin" className="text-section">
            {m.adminTitle}
          </h2>
          <Link to="/o/$org/admin" params={{ org }} className="text-body">
            {m.adminLink}
          </Link>
        </section>
      )}
    </section>
  );
}
