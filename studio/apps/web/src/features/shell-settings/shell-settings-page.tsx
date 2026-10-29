// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import type { ShellChange } from "@plux/api";
import { limitDefault } from "@plux/schema";
import { StatusPill } from "@plux/ui";
import { useParams } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { useCan } from "../admin/use-admin.ts";
import { ProjectDownload } from "./project-download.tsx";
import { ShellForm } from "./shell-form.tsx";
import { shellSettingsMessages } from "./shell-settings.messages.ts";
import { useShellStatus } from "./use-shell.ts";

type Messages = ReturnType<typeof shellSettingsMessages>;

/** The name of a setting as the server reports it in a change. */
function fieldName(m: Messages, field: string): string {
  const names: Readonly<Record<string, string>> = {
    name: m.fieldName,
    package: m.fieldPackage,
    applicationId: m.fieldApplicationId,
    bundleId: m.fieldBundleId,
    appVersion: m.fieldAppVersion,
    splashColor: m.fieldSplashColor,
    icon: m.fieldIcon,
    router: m.fieldRouter,
    deviceApis: m.fieldDeviceApis,
    push: m.fieldPush,
    deepLinkHosts: m.fieldDeepLinkHosts,
    deepLinkSchemes: m.fieldDeepLinkSchemes,
    packages: m.fieldPackages,
    entryRoute: m.fieldEntryRoute,
  };
  return names[field] ?? field;
}

/** A value as the server reports it in a change: lists joined by commas, switches as `true` and `false`. */
function valueText(m: Messages, field: string, value: string): string {
  if (field === "push") {
    return value === "true" ? m.on : m.off;
  }
  if (field === "icon") {
    return value === "default" ? m.iconDefault : value.slice(0, 12);
  }
  return value === "" ? m.none : value;
}

function Changes({ changes }: { readonly changes: readonly ShellChange[] }): ReactElement {
  const m = shellSettingsMessages(useLingui().i18n);
  return (
    <section aria-labelledby="shell-changes" className="flex flex-col gap-2">
      <h2 id="shell-changes" className="text-section">
        {m.changesTitle}
      </h2>
      <p className="text-body text-secondary">{m.requiredCaption}</p>
      <table className="w-full text-start text-body">
        <caption className="sr-only">{m.changesTable}</caption>
        <thead>
          <tr>
            {[m.colSetting, m.colGenerated, m.colCurrent, m.colReason].map((head) => (
              <th key={head} scope="col" className="p-2 text-start text-caption text-secondary">
                {head}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {changes.map((change) => (
            <tr key={change.field} className="border-t border-hairline">
              <th scope="row" className="p-2 text-start font-medium">
                {fieldName(m, change.field)}
              </th>
              <td className="p-2 font-mono text-data">{valueText(m, change.field, change.oldValue)}</td>
              <td className="p-2 font-mono text-data">{valueText(m, change.field, change.newValue)}</td>
              <td className="p-2 text-secondary">{change.reason}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

/** A38: the native shell settings of an app, "shell update required" with exactly what changed, and the project download (GEN-004, F18). */
export function ShellSettingsPage(): ReactElement {
  const { i18n } = useLingui();
  const m = shellSettingsMessages(i18n);
  const appId = (useParams({ strict: false }) as { app?: string }).app ?? "";
  const status = useShellStatus(appId);
  const canManage = useCan("app.manage");
  const data = status.data;
  const generatedAt = data?.generatedAt === undefined ? undefined : timestampDate(data.generatedAt);
  return (
    <section aria-labelledby="shell-title" className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <h1 id="shell-title" className="text-screen">
          {m.title}
        </h1>
        <p className="text-body text-secondary">{m.caption}</p>
      </div>
      {status.isPending && (
        <p role="status" className="text-body text-secondary">
          {m.loading}
        </p>
      )}
      {status.isError && (
        <p role="alert" className="text-body text-danger">
          {m.loadFailed}
        </p>
      )}
      {data !== undefined && (
        <>
          <div className="flex flex-wrap items-center gap-3">
            {data.updateRequired ? (
              <StatusPill status="warning" label={m.statusRequired} value={String(data.changes.length)} />
            ) : data.generated === undefined ? (
              <StatusPill status="neutral" label={m.statusNever} />
            ) : (
              <StatusPill status="success" label={m.statusCurrent} />
            )}
            {generatedAt !== undefined && (
              <p className="text-caption text-secondary">
                {m.generatedAt(i18n.date(generatedAt, { dateStyle: "medium", timeStyle: "short" }))}
              </p>
            )}
          </div>
          {data.updateRequired && <Changes changes={data.changes} />}
          {data.current !== undefined && <ShellForm appId={appId} current={data.current} canManage={canManage} />}
          <section aria-labelledby="shell-project" className="flex flex-col gap-2">
            <h2 id="shell-project" className="text-section">
              {m.projectTitle}
            </h2>
            <p className="text-body text-secondary">{m.projectCaption(limitDefault("project.jobsPerHour"))}</p>
            <div className="flex flex-wrap items-start gap-3">
              <ProjectDownload appId={appId} label={m.projectRegenerate} disabled={!canManage} />
              {data.generated !== undefined && data.updateRequired && (
                <ProjectDownload appId={appId} label={m.projectPatch} format="patch" disabled={!canManage} />
              )}
            </div>
            {data.generated !== undefined && data.updateRequired && (
              <p className="text-caption text-secondary">{m.projectPatchCaption}</p>
            )}
          </section>
        </>
      )}
    </section>
  );
}
