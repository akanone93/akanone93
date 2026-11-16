// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { type App, AppService, MetadataUpdateSchema } from "@plux/api";
import { Button, Field, Icon, type IconName, Input, StatusPill, Textarea } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { type FormEvent, type ReactElement, type ReactNode, useState } from "react";
import { PageHeader, PageStatus } from "../../components/page.tsx";
import { reasonOf } from "../../components/refusal.tsx";
import { isName } from "../../lib/keys.ts";
import { useApp } from "../../lib/use-app.tsx";
import { useCan } from "../admin/use-admin.ts";
import { shellSettingsMessages } from "../shell-settings/shell-settings.messages.ts";
import { useShellStatus } from "../shell-settings/use-shell.ts";
import { appsMessages } from "./apps.messages.ts";
import { LimitsSummary, LimitsTable } from "./limits-usage.tsx";
import { PluginUsage } from "./plugin-usage.tsx";
import { useRefreshApps } from "./use-apps.ts";
import "./apps.css";

type SaveState = "idle" | "saved" | "failed";

/**
 * The details of the app (N4): its name and its fixed key side by side, its description under them. Save and Discard
 * appear while something differs from what is saved, so the card reads as settled the rest of the time. Saving needs
 * `app.manage` (UpdateApp); everyone who can read the app sees the details.
 */
function DetailsCard({ app, canEdit }: { readonly app: App; readonly canEdit: boolean }): ReactElement {
  const m = appsMessages(useLingui().i18n);
  const { session } = useApp();
  const refresh = useRefreshApps();
  const [name, setName] = useState(app.name);
  const [description, setDescription] = useState(app.description);
  const [attempted, setAttempted] = useState(false);
  const [working, setWorking] = useState(false);
  const [state, setState] = useState<SaveState>("idle");
  const [reason, setReason] = useState("");
  const dirty = name !== app.name || description !== app.description;
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
      setReason("");
      await refresh();
    } catch (e) {
      setState("failed");
      setReason(reasonOf(e));
    } finally {
      setWorking(false);
    }
  };
  const discard = () => {
    setName(app.name);
    setDescription(app.description);
    setAttempted(false);
    setState("idle");
  };
  return (
    <section aria-labelledby="app-settings-details" className="settings-card">
      <h2 id="app-settings-details" className="text-section">
        {m.detailsTitle}
      </h2>
      <form noValidate onSubmit={(e) => void submit(e)} className="settings-form">
        <div className="settings-name-row">
          <Field label={m.nameLabel} error={attempted && !isName(name) ? m.nameInvalid : undefined}>
            <Input
              value={name}
              disabled={!canEdit}
              autoComplete="off"
              onChange={(e) => {
                setName(e.target.value);
                setState("idle");
              }}
            />
          </Field>
          <Field label={m.keyLabel} description={m.keyHint}>
            <Input value={app.key} readOnly spellCheck={false} />
          </Field>
        </div>
        <Field label={m.descriptionLabel}>
          <Textarea
            value={description}
            disabled={!canEdit}
            rows={3}
            onChange={(e) => {
              setDescription(e.target.value);
              setState("idle");
            }}
          />
        </Field>
        {!canEdit && <p className="text-body text-muted-foreground">{m.readOnly}</p>}
        {canEdit && (dirty || state !== "idle" || attempted) && (
          <div className="flex flex-wrap items-center gap-3">
            <Button type="submit" loading={working} disabled={!dirty && !attempted}>
              {m.save}
            </Button>
            {dirty && (
              <Button type="button" variant="ghost" disabled={working} onClick={discard}>
                {m.discard}
              </Button>
            )}
            {dirty && state === "idle" && <span className="text-caption text-muted-foreground">{m.unsaved}</span>}
            {state === "saved" && !dirty && (
              <p role="status" className="text-body text-muted-foreground">
                {m.saved}
              </p>
            )}
          </div>
        )}
        {state === "failed" && (
          <p role="alert" className="text-body text-danger">
            {m.saveFailed}
            {reason !== "" && <span className="block text-muted-foreground">{reason}</span>}
          </p>
        )}
      </form>
    </section>
  );
}

/** A card that opens another settings page: what it is, in a line, and where it goes; its link names the action. */
function SettingsCard(props: {
  readonly id: string;
  readonly icon: IconName;
  readonly title: string;
  readonly caption: string;
  readonly link: ReactNode;
  readonly badge?: ReactNode;
}): ReactElement {
  return (
    <li className="settings-link-card">
      <span className="settings-link-icon" aria-hidden="true">
        <Icon name={props.icon} />
      </span>
      <span className="flex min-w-0 flex-col gap-1">
        <span className="flex flex-wrap items-center gap-2">
          <span id={`${props.id}-title`} className="font-medium">
            {props.title}
          </span>
          {props.badge}
        </span>
        <span id={`${props.id}-caption`} className="text-caption text-muted-foreground">
          {props.caption}
        </span>
      </span>
      {props.link}
    </li>
  );
}

/** The other settings of the app, as cards (N4): the native shell, the host fonts and, for those who may, the organisation. */
function ConfigureCards({ org, appId, canAdmin }: { org: string; appId: string; canAdmin: boolean }): ReactElement {
  const m = appsMessages(useLingui().i18n);
  const shellText = shellSettingsMessages(useLingui().i18n);
  const shell = useShellStatus(appId);
  const linkClass = "settings-link-stretch";
  return (
    <section aria-labelledby="app-settings-configure" className="flex flex-col gap-3">
      <h2 id="app-settings-configure" className="text-section">
        {m.configureTitle}
      </h2>
      <ul className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        <SettingsCard
          id="app-settings-shell"
          icon="device-mobile"
          title={m.shellTitle}
          caption={m.shellCaption}
          badge={
            shell.data?.updateRequired === true && <StatusPill status="warning" label={shellText.statusRequired} />
          }
          link={
            <Link
              to="/o/$org/apps/$app/settings/shell"
              params={{ org, app: appId }}
              aria-label={shellText.linkLabel}
              aria-describedby="app-settings-shell-caption"
              className={linkClass}
            />
          }
        />
        <SettingsCard
          id="app-settings-fonts"
          icon="text-t"
          title={m.fontsTitle}
          caption={m.fontsCaption}
          link={
            <Link
              to="/o/$org/apps/$app/design/typography"
              params={{ org, app: appId }}
              aria-label={m.fontsLink}
              aria-describedby="app-settings-fonts-caption"
              className={linkClass}
            />
          }
        />
        {canAdmin && (
          <SettingsCard
            id="app-settings-admin"
            icon="users"
            title={m.adminTitle}
            caption={m.adminCaption}
            link={
              <Link
                to="/o/$org/admin"
                params={{ org }}
                aria-label={m.adminLink}
                aria-describedby="app-settings-admin-caption"
                className={linkClass}
              />
            }
          />
        )}
      </ul>
    </section>
  );
}

/** What the app uses of its limits (N4): the headline meters, and every limit in a table behind a disclosure. */
function UsageSection({ appId }: { readonly appId: string }): ReactElement {
  const m = appsMessages(useLingui().i18n);
  const { session } = useApp();
  const limits = useQuery({
    queryKey: ["app-limits", appId],
    queryFn: async () => (await createClient(AppService, session.transport).listAppLimits({ appId })).limits,
  });
  return (
    <section aria-labelledby="app-settings-limits" className="settings-card">
      <div className="flex flex-col gap-1">
        <h2 id="app-settings-limits" className="text-section">
          {m.limitsTitle}
        </h2>
        <p className="text-body text-muted-foreground">{m.limitsCaption}</p>
      </div>
      {limits.isError && (
        <p role="alert" className="text-body text-danger">
          {m.limitsLoadFailed}
        </p>
      )}
      {limits.isSuccess && (
        <>
          <LimitsSummary limits={limits.data} />
          <details className="settings-more">
            <summary>{m.allLimits(limits.data.length)}</summary>
            <LimitsTable limits={limits.data} />
          </details>
        </>
      )}
    </section>
  );
}

/**
 * The settings of one app (STU-023, STU-025, LIM-005, N4), in the order they are used: its details, the cards that
 * open its other settings, what it uses of its limits, and, last, what each plugin uses.
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
  const loaded = app.data;
  return (
    <section aria-labelledby="app-settings-title" className="page-frame">
      <PageHeader id="app-settings-title" title={m.settingsTitle(loaded?.name ?? "")} caption={m.settingsCaption} />
      <div className="page-body app-settings">
        <PageStatus
          loading={app.isPending}
          failed={app.isError}
          error={app.error}
          labels={{ loading: m.loading, loadFailed: m.settingsLoadFailed }}
        />
        {loaded !== undefined && <DetailsCard key={loaded.id} app={loaded} canEdit={canEdit} />}
        <ConfigureCards org={org} appId={appId} canAdmin={canAdmin} />
        <UsageSection appId={appId} />
        {canPlugins && <PluginUsage appId={appId} />}
      </div>
    </section>
  );
}
