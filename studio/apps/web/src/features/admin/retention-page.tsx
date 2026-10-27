// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { AnalyticsService } from "@plux/api";
import type { ReactElement } from "react";
import { adminMessages } from "./admin.messages.ts";
import { CreateForm, Frame, fmt, Table } from "./parts.tsx";
import { useAdminQuery, useCan, useRpc, useRun } from "./use-admin.ts";

/** O29: how long each data class is kept; a change waits for its notice period unless applied at once (STU-116). */
export function RetentionPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const analytics = useRpc(AnalyticsService);
  const can = useCan("retention.manage");
  const { run, failed } = useRun();
  const retention = useAdminQuery(["retention"], () => analytics.getRetention({}));
  const settings = retention.data?.settings ?? [];
  return (
    <Frame
      id="admin-retention"
      title={m.retentionTitle}
      caption={m.retentionCaption}
      loading={retention.isPending}
      failed={retention.isError}
      saveFailed={failed}
    >
      <Table
        caption={m.retentionTitle}
        heads={[m.colClass, m.colDays, m.colDefault]}
        empty={m.nothing}
        rows={settings.map((s) => ({
          key: s.dataClass,
          cells: [s.dataClass, String(s.days), s.isDefault ? m.yes : m.no],
        }))}
      />
      <CreateForm
        fields={[
          {
            name: "dataClass",
            label: m.colClass,
            options: settings.map((s) => ({ value: s.dataClass, label: s.dataClass })),
          },
          { name: "days", label: m.setDays, type: "number", required: true },
          {
            name: "immediate",
            label: m.applyNow,
            options: [
              { value: "no", label: m.no },
              { value: "yes", label: m.yes },
            ],
          },
        ]}
        submit={m.save}
        disabled={!can || settings.length === 0}
        deniedReason={m.noPermission}
        onSubmit={(v) =>
          run(() =>
            analytics.setRetention({
              dataClass: v("dataClass"),
              days: Number(v("days")),
              immediate: v("immediate") === "yes",
            }),
          )
        }
      />
      <h2 className="text-label font-medium">{m.scheduledChanges}</h2>
      <Table
        caption={m.scheduledChanges}
        heads={[m.colClass, m.colDays, m.colState, m.colPeriod]}
        empty={m.nothing}
        rows={(retention.data?.scheduled ?? []).map((c) => ({
          key: c.id,
          cells: [c.dataClass, String(c.days), c.state, fmt(c.applyAt, m.never)],
        }))}
      />
    </Frame>
  );
}
