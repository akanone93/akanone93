// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import { ComplianceService } from "@plux/api";
import { Button } from "@plux/ui";
import type { ReactElement } from "react";
import { adminMessages } from "./admin.messages.ts";
import { download } from "./download.ts";
import { CreateForm, Frame, fmt, Table } from "./parts.tsx";
import { pageSize, useAdminQuery, useCan, useRpc, useRun } from "./use-admin.ts";

/** O28: signed compliance reports of the changes in a period (GOV-026). */
export function CompliancePage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const compliance = useRpc(ComplianceService);
  const can = useCan("compliance.report");
  const { run, failed } = useRun();
  const reports = useAdminQuery(["compliance"], () => compliance.listComplianceReports({ page: { pageSize } }));
  const fetchReport = async (id: string) => {
    const { content, contentType } = await compliance.getComplianceReport({ id, format: "json" });
    download(`compliance-${id}.json`, new Uint8Array(content), contentType || "application/json");
  };
  return (
    <Frame
      id="admin-compliance"
      title={m.complianceTitle}
      caption={m.complianceCaption}
      loading={reports.isPending}
      failed={reports.isError}
      saveFailed={failed}
    >
      <Table
        caption={m.complianceTitle}
        heads={[m.colPeriod, m.colChanges, m.colSigned, m.colCreated, ""]}
        empty={m.nothing}
        rows={(reports.data?.reports ?? []).map((r) => ({
          key: r.id,
          cells: [
            `${fmt(r.periodFrom, m.never)} - ${fmt(r.periodTo, m.never)}`,
            String(r.changes),
            r.signed ? m.yes : m.no,
            fmt(r.requestedAt, m.never),
            <Button key="d" size="sm" variant="secondary" onClick={() => run(() => fetchReport(r.id))}>
              {m.download}
            </Button>,
          ],
        }))}
      />
      <CreateForm
        fields={[
          { name: "from", label: m.periodFrom, type: "date", required: true },
          { name: "to", label: m.periodTo, type: "date", required: true },
        ]}
        submit={m.createReport}
        disabled={!can}
        deniedReason={m.noPermission}
        onSubmit={(v) =>
          run(() =>
            compliance.createComplianceReport({
              from: timestampFromDate(new Date(v("from"))),
              to: timestampFromDate(new Date(v("to"))),
            }),
          )
        }
      />
    </Frame>
  );
}
