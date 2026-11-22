// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { AppService } from "@plux/api";
import { Button, Field, Select, Tooltip } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { adminMessages } from "./admin.messages.ts";
import { CreateForm, Frame, fmt, Table } from "./parts.tsx";
import { pageSize, useAdminQuery, useCan, useOrgId, useRpc, useRun } from "./use-admin.ts";

/**
 * O32: the environments of each app and their secrets (STU-116, SEC-106). A secret's value is written once and never
 * shown again, only the last characters the server keeps as a hint; setting or deleting one asks for the security key
 * first (SEC-103), which the session does when the server refuses without it.
 */
export function EnvironmentsPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const organizationId = useOrgId();
  const apps = useRpc(AppService);
  const canEnvironments = useCan("environment.manage");
  const canSecrets = useCan("secrets.manage");
  const { run, failed, error } = useRun();
  const [chosenApp, setChosenApp] = useState<string>();
  const [chosenEnv, setChosenEnv] = useState<string>();
  const appList = useAdminQuery(["apps", organizationId], () => apps.listApps({ organizationId, page: { pageSize } }));
  const appId = chosenApp ?? appList.data?.apps[0]?.id ?? "";
  const envList = useAdminQuery(
    ["environments", appId],
    () => apps.listEnvironments({ appId, page: { pageSize } }),
    appId !== "",
  );
  const environments = envList.data?.environments ?? [];
  const environment =
    environments.find((e) => e.id === chosenEnv) ??
    environments.find((e) => e.key === "development") ??
    environments[0];
  const environmentId = environment?.id ?? "";
  const environmentName = environment === undefined ? "" : environment.name || environment.key;
  const secrets = useAdminQuery(
    ["secrets", environmentId],
    () => apps.listSecrets({ environmentId, page: { pageSize } }),
    environmentId !== "",
  );
  return (
    <Frame
      id="admin-environments"
      title={m.environmentsTitle}
      caption={m.environmentsCaption}
      loading={appList.isPending}
      failed={appList.isError || envList.isError || secrets.isError}
      error={appList.error ?? envList.error ?? secrets.error}
      saveFailed={failed}
      saveError={error}
    >
      <Field label={m.environmentsApp}>
        <Select
          label={m.environmentsApp}
          options={(appList.data?.apps ?? []).map((a) => ({ value: a.id, label: a.name }))}
          value={appId}
          onValueChange={(v) => {
            setChosenApp(v);
            setChosenEnv(undefined);
          }}
        />
      </Field>
      <Table
        caption={m.environmentsTitle}
        heads={[m.colKey, m.colName, m.environmentsProduction, ""]}
        empty={m.nothing}
        rows={environments.map((e) => ({
          key: e.id,
          cells: [
            <span key="k" className="font-mono">
              {e.key}
            </span>,
            e.name,
            e.production ? m.yes : m.no,
            <Button
              key="s"
              size="sm"
              variant={e.id === environmentId ? "secondary" : "ghost"}
              aria-pressed={e.id === environmentId}
              onClick={() => setChosenEnv(e.id)}
            >
              {m.environmentsShowSecrets}
            </Button>,
          ],
        }))}
      />
      <CreateForm
        fields={[
          { name: "key", label: m.colKey, required: true },
          { name: "name", label: m.colName, required: true },
          {
            name: "production",
            label: m.environmentsProduction,
            options: [
              { value: "no", label: m.no },
              { value: "yes", label: m.yes },
            ],
          },
        ]}
        submit={m.environmentsCreate}
        disabled={!canEnvironments || appId === ""}
        deniedReason={m.noPermission}
        onSubmit={(v) =>
          run(() =>
            apps.createEnvironment({ appId, key: v("key"), name: v("name"), production: v("production") === "yes" }),
          )
        }
      />
      <h2 className="text-label font-medium">{m.secretsOf(environmentName)}</h2>
      <p className="text-body text-muted-foreground">{m.secretsNote}</p>
      <Table
        caption={m.secretsOf(environmentName)}
        heads={[m.colKey, m.secretsHint, m.secretsUpdated, ""]}
        empty={m.nothing}
        rows={(secrets.data?.secrets ?? []).map((s) => ({
          key: s.key,
          cells: [
            <span key="k" className="font-mono">
              {s.key}
            </span>,
            <span key="h" className="font-mono">
              {s.hint}
            </span>,
            fmt(s.updatedAt, m.never),
            <Tooltip key="d" content={canSecrets ? undefined : m.noPermission} disabledTrigger={!canSecrets}>
              <Button
                size="sm"
                variant="danger"
                disabled={!canSecrets}
                onClick={() => run(() => apps.deleteSecret({ environmentId, key: s.key }))}
              >
                {m.delete}
              </Button>
            </Tooltip>,
          ],
        }))}
      />
      <CreateForm
        fields={[
          { name: "key", label: m.colKey, required: true },
          { name: "value", label: m.secretsValue, type: "password", required: true },
        ]}
        submit={m.secretsSet}
        disabled={!canSecrets || environmentId === ""}
        deniedReason={m.noPermission}
        onSubmit={(v) => run(() => apps.setSecret({ environmentId, key: v("key"), value: v("value") }))}
      />
    </Frame>
  );
}
