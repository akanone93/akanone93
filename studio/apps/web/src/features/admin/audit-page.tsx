// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { AuditService, IdentityService } from "@plux/api";
import { Button, Field, Input, Tooltip } from "@plux/ui";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { Refusal } from "../../components/refusal.tsx";
import { adminMessages } from "./admin.messages.ts";
import type { AuditSearch } from "./admin.schema.ts";
import { DestinationForm } from "./audit-destinations.tsx";
import { auditCsv, keepEntry, serverFilter } from "./audit-model.ts";
import { download } from "./download.ts";
import { Frame, fmt, Table } from "./parts.tsx";
import { pageSize, useAdminQuery, useCan, useOrgId, useRpc, useRun } from "./use-admin.ts";

const filters = ["from", "to", "actor", "action", "resource"] as const;

/** The audit log's ordering with today's changes on the first page (SRV-004). */
const newestFirst = "sequence desc";

/** O27: the organisation's audit log, newest first, with filters in the URL and a CSV export of what is shown (STU-116). */
export function AuditPage(): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const organizationId = useOrgId();
  const org = (useParams({ strict: false }) as { org?: string }).org ?? "";
  const search = useSearch({ strict: false }) as AuditSearch;
  const navigate = useNavigate();
  const identity = useRpc(IdentityService);
  const audit = useRpc(AuditService);
  const canExport = useCan("audit.export");
  const { run, failed: saveFailed, error: saveError } = useRun();
  const [draft, setDraft] = useState<AuditSearch>(search);
  const filter = serverFilter(search.action, search.resource);
  const entries = useInfiniteQuery({
    queryKey: ["admin", "audit", organizationId, filter],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      identity.listAuditEntries({
        organizationId,
        page: { pageSize, pageToken: pageParam, filter, orderBy: newestFirst },
      }),
    getNextPageParam: (last) => last.page?.nextPageToken || undefined,
  });
  const destinations = useAdminQuery(["export-destinations"], () =>
    audit.listExportDestinations({ page: { pageSize } }),
  );
  const shown = (entries.data?.pages ?? [])
    .flatMap((p) => p.entries)
    .filter((e) => keepEntry(e, search.from, search.to, search.actor));
  const labels = [m.filterFrom, m.filterTo, m.filterActor, m.filterAction, m.filterResource];
  return (
    <Frame
      id="admin-audit"
      title={m.auditTitle}
      caption={m.auditCaption}
      loading={entries.isPending}
      failed={entries.isError}
      error={entries.error}
      saveFailed={saveFailed}
      saveError={saveError}
    >
      <form
        className="flex flex-wrap items-end gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          navigate({ to: "/o/$org/admin/audit", params: { org }, search: draft });
        }}
      >
        {filters.map((name, i) => (
          <Field key={name} label={labels[i] ?? name}>
            <Input
              type={name === "from" || name === "to" ? "date" : "text"}
              value={draft[name] ?? ""}
              onChange={(e) => setDraft({ ...draft, [name]: e.target.value === "" ? undefined : e.target.value })}
            />
          </Field>
        ))}
        <Button type="submit">{m.apply}</Button>
      </form>
      <Table
        caption={m.auditTitle}
        heads={[m.colTime, m.colActor, m.colAction, m.colResource]}
        empty={m.nothing}
        rows={shown.map((e) => ({
          key: e.id,
          cells: [
            fmt(e.time, m.never),
            e.actor?.display || e.actor?.id || "",
            e.action,
            `${e.targetKind} ${e.targetId}`,
          ],
        }))}
      />
      <div className="flex gap-3">
        {entries.hasNextPage && (
          <Button variant="secondary" onClick={() => entries.fetchNextPage()}>
            {m.moreEntries}
          </Button>
        )}
        <Tooltip content={canExport ? undefined : m.noPermission} disabledTrigger={!canExport}>
          <Button
            disabled={!canExport}
            onClick={() => download("audit.csv", new TextEncoder().encode(auditCsv(shown)), "text/csv")}
          >
            {m.exportCsv}
          </Button>
        </Tooltip>
      </div>
      <h2 className="text-label font-medium">{m.destinations}</h2>
      {/* The destinations need a recent step-up and audit.export; a refusal says so rather than showing none. */}
      {destinations.isError ? (
        <Refusal error={destinations.error} failedTitle={m.loadFailed} />
      ) : (
        <Table
          caption={m.destinations}
          heads={[m.colName, m.colKind, m.colEnabled, m.colLastUsed]}
          empty={m.nothing}
          rows={(destinations.data?.destinations ?? []).map((d) => ({
            key: d.id,
            cells: [d.name, m.destinationKind(d.kind), d.enabled ? m.yes : m.no, fmt(d.lastSuccessAt, m.never)],
          }))}
        />
      )}
      <DestinationForm can={canExport} run={run} />
    </Frame>
  );
}
