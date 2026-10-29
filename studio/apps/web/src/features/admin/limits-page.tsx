// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { OrgService } from "@plux/api";
import { Button, Field, Input } from "@plux/ui";
import { type ReactElement, useState } from "react";
import { adminMessages } from "./admin.messages.ts";
import { Frame, Table } from "./parts.tsx";
import { useAdminQuery, useCan, useOrgId, useRpc, useRun } from "./use-admin.ts";

function Override({
  limit,
  disabled,
  onSave,
}: {
  readonly limit: { readonly key: string; readonly value: bigint; readonly hardMax: bigint };
  readonly disabled: boolean;
  readonly onSave: (value: bigint) => void;
}): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const [text, setText] = useState(String(limit.value));
  const value = /^\d+$/.test(text) ? BigInt(text) : undefined;
  const valid = value !== undefined && (limit.hardMax === 0n || value <= limit.hardMax);
  return (
    <span className="flex items-end gap-2">
      <Field label={`${m.colOverride} ${limit.key}`}>
        <Input inputMode="numeric" value={text} disabled={disabled} onChange={(e) => setText(e.target.value)} />
      </Field>
      <Button size="sm" disabled={disabled || !valid} onClick={() => value !== undefined && onSave(value)}>
        {m.save}
      </Button>
    </span>
  );
}

/** O31: the organisation's limits from the limits registry with its overrides and usage (LIM-001, STU-025). */
export function LimitsPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const organizationId = useOrgId();
  const org = useRpc(OrgService);
  const can = useCan("limits.manage");
  const { run, failed } = useRun();
  const limits = useAdminQuery(["limits", organizationId], () => org.listOrganizationLimits({ organizationId }));
  return (
    <Frame
      id="admin-limits"
      title={m.limitsTitle}
      caption={m.limitsCaption}
      loading={limits.isPending}
      failed={limits.isError}
      saveFailed={failed}
    >
      <Table
        caption={m.limitsTitle}
        heads={[m.colLimit, m.colEffective, m.colMax, m.colOverride]}
        empty={m.nothing}
        rows={(limits.data?.limits ?? []).map((l) => ({
          key: l.key,
          cells: [
            <span key="k" className="font-mono">
              {l.key}
              {l.warning ? ` (${m.limitWarning})` : ""}
            </span>,
            `${l.effective} ${l.unit}`,
            l.hardMax === 0n ? "" : String(l.hardMax),
            <Override
              key="o"
              limit={l}
              disabled={!can}
              onSave={(value) => run(() => org.setOrganizationLimit({ organizationId, key: l.key, value }))}
            />,
          ],
        }))}
      />
    </Frame>
  );
}
