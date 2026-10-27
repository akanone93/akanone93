// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Select } from "@plux/ui";
import { useNavigate, useSearch } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { approvalsMessages } from "./approvals.messages.ts";
import { decidedStatuses } from "./approvals.schema.ts";
import { AppFilter, Frame, RequestTable } from "./parts.tsx";
import { useApprovals, useMe, useScope } from "./use-approvals.ts";

const anyStatus = "*";

/** O7: the requests waiting for a decision, filtered by `?app&status&mine` (STU-102). */
export function InboxPage(): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const { app } = useScope();
  const search = useSearch({ strict: false }) as { status?: string; mine?: boolean };
  const status = search.status ?? "open";
  const me = useMe();
  const navigate = useNavigate();
  const requests = useApprovals(app, search.status === undefined ? "open" : status);
  const shown = (requests.data?.approvals ?? []).filter((r) => search.mine !== true || r.requestedBy === me.id);
  const names: Record<string, string> = {
    open: m.statusOpen,
    approved: m.statusApproved,
    rejected: m.statusRejected,
    expired: m.statusExpired,
    superseded: m.statusSuperseded,
    used: m.statusUsed,
  };
  const change = (patch: object) =>
    void navigate({ to: ".", replace: true, search: (prev: object) => ({ ...prev, ...patch }) } as never);
  return (
    <Frame
      id="approvals-title"
      title={m.inboxTitle}
      caption={m.inboxCaption}
      loading={requests.isPending}
      failed={requests.isError}
    >
      <div className="flex items-end gap-4">
        <AppFilter />
        <Select
          label={m.statusFilter}
          options={["open", ...decidedStatuses].map((value) => ({ value, label: names[value] ?? value }))}
          value={status === "" ? anyStatus : status}
          onValueChange={(value) => change({ status: value === "open" ? undefined : value })}
        />
        <label className="flex items-center gap-2 text-body">
          <input
            type="checkbox"
            checked={search.mine === true}
            onChange={(e) => change({ mine: e.target.checked ? true : undefined })}
          />
          {m.mineOnly}
        </label>
      </div>
      <RequestTable requests={shown} />
    </Frame>
  );
}
