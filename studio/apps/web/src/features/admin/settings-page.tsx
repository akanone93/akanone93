// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { OrgService } from "@plux/api";
import { Button, Field, Input } from "@plux/ui";
import { type ReactElement, useEffect, useState } from "react";
import { adminMessages } from "./admin.messages.ts";
import { Frame } from "./parts.tsx";
import { useAdminQuery, useCan, useOrgId, useRpc, useRun } from "./use-admin.ts";

/** O15: the organisation's name, with the area's navigation (STU-116). */
export function SettingsPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const id = useOrgId();
  const org = useRpc(OrgService);
  const can = useCan("organization.manage");
  const { run, failed } = useRun();
  const current = useAdminQuery(["organisation", id], () => org.getOrganization({ id }));
  const [name, setName] = useState("");
  const [done, setDone] = useState(false);
  const stored = current.data?.organization?.name;
  useEffect(() => {
    if (stored !== undefined) {
      setName(stored);
    }
  }, [stored]);
  return (
    <Frame
      id="admin-settings"
      title={m.settingsTitle}
      caption={m.settingsCaption}
      loading={current.isPending}
      failed={current.isError}
      saveFailed={failed}
    >
      <form
        className="flex items-end gap-3"
        onSubmit={async (e) => {
          e.preventDefault();
          setDone(await run(() => org.updateOrganization({ id, name })));
        }}
      >
        <Field label={m.orgName}>
          <Input value={name} required onChange={(e) => setName(e.target.value)} />
        </Field>
        <Button type="submit" disabled={!can} title={can ? undefined : m.noPermission}>
          {m.save}
        </Button>
      </form>
      {done && (
        <p role="status" className="text-body text-secondary">
          {m.saved}
        </p>
      )}
    </Frame>
  );
}
