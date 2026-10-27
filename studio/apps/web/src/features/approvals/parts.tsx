// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import type { ApprovalRequest } from "@plux/api";
import { Select, StatusPill } from "@plux/ui";
import { Link, useNavigate } from "@tanstack/react-router";
import type { ReactElement, ReactNode } from "react";
import { fmt, Table } from "../security/parts.tsx";
import { approvalsMessages } from "./approvals.messages.ts";
import { useApps, useScope } from "./use-approvals.ts";

const allApps = "*";

/** Heading, sub-navigation, and the loading and error states shared by the pages of the area. */
export function Frame({
  id,
  title,
  caption,
  loading,
  failed,
  children,
}: {
  readonly id: string;
  readonly title: string;
  readonly caption?: string;
  readonly loading: boolean;
  readonly failed: boolean;
  readonly children: ReactNode;
}): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  return (
    <section className="flex flex-col gap-4" aria-labelledby={id}>
      <h1 id={id} className="text-screen">
        {title}
      </h1>
      {caption !== undefined && <p className="text-body text-secondary">{caption}</p>}
      <AreaNav />
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

/** The links between O7, O9, O10 and O11, which keep the chosen app. */
function AreaNav(): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const { org, app } = useScope();
  const search = app === "" ? {} : { app };
  const links = [
    { to: "/o/$org/approvals", label: m.navInbox },
    { to: "/o/$org/approvals/history", label: m.navHistory },
    { to: "/o/$org/approvals/break-glass", label: m.navBreakGlass },
    { to: "/o/$org/approvals/policies", label: m.navPolicies },
  ];
  return (
    <nav className="flex gap-4" aria-label={m.inboxTitle}>
      {links.map((l) => (
        <Link key={l.to} to={l.to} params={{ org } as never} search={search as never} className="text-body underline">
          {l.label}
        </Link>
      ))}
    </nav>
  );
}

/** The app filter of `?app`: one app or the whole organisation. */
export function AppFilter(): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const { app } = useScope();
  const apps = useApps();
  const navigate = useNavigate();
  const options = [
    { value: allApps, label: m.allApps },
    ...(apps.data?.apps ?? []).map((a) => ({ value: a.id, label: a.name })),
  ];
  return (
    <Select
      label={m.appFilter}
      options={options}
      value={app === "" ? allApps : app}
      onValueChange={(value) =>
        void navigate({
          to: ".",
          replace: true,
          search: (prev: object) => ({ ...prev, app: value === allApps ? undefined : value }),
        } as never)
      }
    />
  );
}

const pillKinds: Record<string, "success" | "danger" | "neutral" | "warning"> = {
  approved: "success",
  used: "success",
  rejected: "danger",
  open: "warning",
  expired: "neutral",
  superseded: "neutral",
};

/** A request's status as a pill. */
export function Status({ status }: { readonly status: string }): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const names: Record<string, string> = {
    open: m.statusOpen,
    approved: m.statusApproved,
    rejected: m.statusRejected,
    expired: m.statusExpired,
    superseded: m.statusSuperseded,
    used: m.statusUsed,
  };
  return <StatusPill status={pillKinds[status] ?? "neutral"} label={names[status] ?? status} />;
}

/** The requests of O7 and O9, each linking to its O8. */
export function RequestTable({ requests }: { readonly requests: readonly ApprovalRequest[] }): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const { org } = useScope();
  if (requests.length === 0) {
    return <p className="text-body text-secondary">{m.empty}</p>;
  }
  return (
    <Table
      caption={m.inboxTitle}
      heads={[
        m.colSubject,
        m.colResource,
        m.colEnvironment,
        m.colRequester,
        m.colStatus,
        m.colApprovals,
        m.colCreated,
        m.colExpires,
      ]}
      rows={requests.map((r) => ({
        key: r.id,
        cells: [
          <Link
            key="s"
            to="/o/$org/approvals/$requestId"
            params={{ org, requestId: r.id } as never}
            search={{ app: r.appId } as never}
            className="underline"
          >
            {r.summary === "" ? r.subject : r.summary}
          </Link>,
          r.resource,
          r.environment,
          r.requestedBy,
          <Status key="t" status={r.status} />,
          m.approvalsOf({
            given: String(r.decisions.filter((d) => d.decision === "approve").length),
            needed: String(r.approvalsRequired),
          }),
          fmt(r.createdAt === undefined ? undefined : timestampDate(r.createdAt), m.never),
          fmt(r.expiresAt === undefined ? undefined : timestampDate(r.expiresAt), m.never),
        ],
      }))}
    />
  );
}
