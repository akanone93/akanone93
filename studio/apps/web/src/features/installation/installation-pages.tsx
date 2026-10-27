// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { IdentityService, InstallationService, OrgService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import type { ReactElement, ReactNode } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { installationMessages } from "./installation.messages.ts";

type Messages = ReturnType<typeof installationMessages>;

function Nav({ m }: { readonly m: Messages }): ReactElement {
  const items = [
    { to: "/installation", label: m.navHealth },
    { to: "/installation/organisations", label: m.navOrganisations },
    { to: "/installation/storage", label: m.navStorage },
    { to: "/installation/jobs", label: m.navJobs },
    { to: "/installation/backups", label: m.navBackups },
    { to: "/installation/issuers", label: m.navIssuers },
    { to: "/installation/audit", label: m.navAudit },
  ] as const;
  return (
    <nav aria-label={m.navLabel} className="flex flex-wrap gap-3 text-body">
      {items.map((i) => (
        <Link key={i.to} to={i.to} activeOptions={{ exact: true }}>
          {i.label}
        </Link>
      ))}
    </nav>
  );
}

function Frame({
  id,
  title,
  caption,
  loading = false,
  failed = false,
  children,
}: {
  readonly id: string;
  readonly title: string;
  readonly caption: string;
  readonly loading?: boolean;
  readonly failed?: boolean;
  readonly children: ReactNode;
}): ReactElement {
  const m = installationMessages(useLingui().i18n);
  return (
    <section className="flex flex-col gap-4" aria-labelledby={id}>
      <Nav m={m} />
      <h1 id={id} className="text-screen">
        {title}
      </h1>
      <p className="text-body text-secondary">{caption}</p>
      {loading && !failed && (
        <p role="status" className="text-body text-secondary">
          {m.loading}
        </p>
      )}
      {failed && (
        <p role="alert" className="text-body text-danger">
          {m.loadFailed}
        </p>
      )}
      {!loading && !failed && children}
    </section>
  );
}

function Table({
  caption,
  heads,
  rows,
  empty,
}: {
  readonly caption: string;
  readonly heads: readonly string[];
  readonly rows: readonly { readonly key: string; readonly cells: readonly ReactNode[] }[];
  readonly empty: string;
}): ReactElement {
  if (rows.length === 0) {
    return <p className="text-body text-secondary">{empty}</p>;
  }
  return (
    <table className="w-full text-left text-body">
      <caption className="sr-only">{caption}</caption>
      <thead>
        <tr>
          {heads.map((h) => (
            <th key={h} scope="col" className="p-2 text-caption text-secondary">
              {h}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.key}>
            {r.cells.map((c, i) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: cells have a fixed order
              <td key={i} className="p-2">
                {c}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function useInstallationQuery<T>(key: string, read: (api: ReturnType<typeof clients>) => Promise<T>) {
  const { session } = useApp();
  return useQuery({ queryKey: ["installation", key], queryFn: () => read(clients(session.transport)) });
}

function clients(transport: Parameters<typeof createClient>[1]) {
  return {
    installation: createClient(InstallationService, transport),
    org: createClient(OrgService, transport),
    identity: createClient(IdentityService, transport),
  };
}

/** I1: whether each part of the installation answers (STU-116, §4.3 #22). */
export function HealthPage(): ReactElement {
  const m = installationMessages(useLingui().i18n);
  const q = useInstallationQuery("health", (c) => c.installation.getInstallationHealth({}));
  return (
    <Frame id="inst-health" title={m.healthTitle} caption={m.healthCaption} loading={q.isPending} failed={q.isError}>
      <Table
        caption={m.healthTitle}
        heads={[m.colCheck, m.colResult]}
        empty={m.nothing}
        rows={(q.data?.checks ?? []).map((c) => ({
          key: c.name,
          cells: [c.name, c.healthy ? m.healthy : m.unhealthy],
        }))}
      />
    </Frame>
  );
}

/** I2: every organisation of the installation, with its size (STU-116, §4.3 #22). */
export function OrganisationsPage(): ReactElement {
  const m = installationMessages(useLingui().i18n);
  const q = useInstallationQuery("organisations", (c) => c.installation.listInstallationOrganizations({}));
  return (
    <Frame
      id="inst-orgs"
      title={m.organisationsTitle}
      caption={m.organisationsCaption}
      loading={q.isPending}
      failed={q.isError}
    >
      <Table
        caption={m.organisationsTitle}
        heads={[m.colName, m.colCreated, m.colMembers, m.colApps]}
        empty={m.nothing}
        rows={(q.data?.organizations ?? []).map((o) => ({
          key: o.id,
          cells: [o.name, new Date(Number(o.createdAtMs)).toLocaleString(), String(o.memberCount), String(o.appCount)],
        }))}
      />
    </Frame>
  );
}

/** I3: the space the database uses. */
export function StoragePage(): ReactElement {
  const m = installationMessages(useLingui().i18n);
  const q = useInstallationQuery("storage", (c) => c.installation.getInstallationStorage({}));
  return (
    <Frame id="inst-storage" title={m.storageTitle} caption={m.storageCaption} loading={q.isPending} failed={q.isError}>
      <Table
        caption={m.storageTitle}
        heads={[m.colArea, m.colBytes]}
        empty={m.nothing}
        rows={(q.data?.areas ?? []).map((a) => ({
          key: a.area,
          cells: [a.area, `${(Number(a.bytes) / 1024 ** 2).toFixed(1)} MiB`],
        }))}
      />
    </Frame>
  );
}

/** I4: the jobs waiting in each queue. */
export function JobsPage(): ReactElement {
  const m = installationMessages(useLingui().i18n);
  const q = useInstallationQuery("jobs", (c) => c.installation.listInstallationJobs({}));
  return (
    <Frame id="inst-jobs" title={m.jobsTitle} caption={m.jobsCaption} loading={q.isPending} failed={q.isError}>
      <Table
        caption={m.jobsTitle}
        heads={[m.colQueue, m.colState, m.colCount]}
        empty={m.nothing}
        rows={(q.data?.jobs ?? []).map((j) => ({
          key: `${j.queue}/${j.state}`,
          cells: [j.queue, j.state, String(j.count)],
        }))}
      />
    </Frame>
  );
}

/** I5: backups; the server records none yet, so the page says so instead of guessing (§4.3 #22). */
export function BackupsPage(): ReactElement {
  const m = installationMessages(useLingui().i18n);
  return (
    <Frame id="inst-backups" title={m.backupsTitle} caption={m.backupsCaption}>
      <p className="text-body text-secondary">{m.backupsNote}</p>
    </Frame>
  );
}

/** I6: the CI issuers the installation trusts (ADR-0026). */
export function IssuersPage(): ReactElement {
  const m = installationMessages(useLingui().i18n);
  const q = useInstallationQuery("issuers", (c) => c.installation.listTrustedIssuers({}));
  return (
    <Frame id="inst-issuers" title={m.issuersTitle} caption={m.issuersCaption} loading={q.isPending} failed={q.isError}>
      <Table
        caption={m.issuersTitle}
        heads={[m.issuer, m.audience, m.subjectPattern]}
        empty={m.nothing}
        rows={(q.data?.issuers ?? []).map((i) => ({
          key: i.issuer,
          cells: [i.issuer, i.audience, i.subjectPattern],
        }))}
      />
    </Frame>
  );
}

/** I7: the installation's own audit chain, newest first. */
export function InstallationAuditPage(): ReactElement {
  const m = installationMessages(useLingui().i18n);
  const q = useInstallationQuery("audit", (c) => c.identity.listAuditEntries({ page: { pageSize: 200 } }));
  return (
    <Frame id="inst-audit" title={m.auditTitle} caption={m.auditCaption} loading={q.isPending} failed={q.isError}>
      <Table
        caption={m.auditTitle}
        heads={[m.colTime, m.colActor, m.colAction, m.colResource]}
        empty={m.nothing}
        rows={[...(q.data?.entries ?? [])].reverse().map((e) => ({
          key: e.id,
          cells: [
            e.time === undefined ? "" : timestampDate(e.time).toLocaleString(),
            e.actor?.display || e.actor?.id || "",
            e.action,
            `${e.targetKind} ${e.targetId}`.trim(),
          ],
        }))}
      />
    </Frame>
  );
}
