// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { LicenseService } from "@plux/api";
import { Button, Field, Input } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import { type ReactElement, useState } from "react";
import { loadUser } from "../../lib/guards.ts";
import { useApp } from "../../lib/use-app.tsx";
import { adminMessages } from "./admin.messages.ts";
import { Frame, fmt } from "./parts.tsx";
import { useAdminQuery, useRpc, useRun } from "./use-admin.ts";

/** O30: the enterprise licence (GOV-030); only an installation administrator uploads one (§4.3 #23). */
export function LicensePage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const { session } = useApp();
  const license = useRpc(LicenseService);
  const { run, failed } = useRun();
  const user = useQuery({ queryKey: ["current-user"], queryFn: () => loadUser(session) });
  const state = useAdminQuery(["license"], () => license.getLicenseState({}));
  const [file, setFile] = useState<File>();
  const s = state.data;
  const rows: readonly (readonly [string, string])[] =
    s === undefined
      ? []
      : [
          [m.licenseState, s.state],
          [m.licenseId, s.licenseId],
          [m.licenseOrg, s.organization],
          [m.colExpires, fmt(s.expiresAt, m.never)],
          [m.licenseFeatures, s.features.join(", ")],
        ];
  return (
    <Frame
      id="admin-license"
      title={m.licenseTitle}
      caption={m.licenseCaption}
      loading={state.isPending}
      failed={state.isError}
      saveFailed={failed}
    >
      <dl className="flex flex-col gap-1 text-body">
        {rows.map(([k, v]) => (
          <div key={k} className="flex gap-4">
            <dt className="w-48 text-secondary">{k}</dt>
            <dd className="font-mono">{v}</dd>
          </div>
        ))}
      </dl>
      {s?.message !== undefined && s.message !== "" && <p className="text-body text-secondary">{s.message}</p>}
      <p className="text-body text-secondary">{m.licenseNote}</p>
      <form
        className="flex items-end gap-3"
        onSubmit={async (e) => {
          e.preventDefault();
          if (file !== undefined) {
            const bytes = new Uint8Array(await file.arrayBuffer());
            await run(() => license.installLicense({ license: bytes }));
          }
        }}
      >
        <Field label={m.licenseFile}>
          <Input type="file" onChange={(e) => setFile(e.target.files?.[0])} />
        </Field>
        <Button
          type="submit"
          disabled={user.data?.installationAdmin !== true || file === undefined}
          title={user.data?.installationAdmin === true ? undefined : m.noPermission}
        >
          {m.licenseUpload}
        </Button>
      </form>
    </Frame>
  );
}
