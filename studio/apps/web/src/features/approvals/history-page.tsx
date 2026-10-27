// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import type { ReactElement } from "react";
import { approvalsMessages } from "./approvals.messages.ts";
import { decidedStatuses } from "./approvals.schema.ts";
import { AppFilter, Frame, RequestTable } from "./parts.tsx";
import { useApprovals, useScope } from "./use-approvals.ts";

/** O9: every request that is no longer waiting, newest first (STU-102). */
export function HistoryPage(): ReactElement {
  const m = approvalsMessages(useLingui().i18n);
  const { app } = useScope();
  const requests = useApprovals(app, "");
  const decided = (requests.data?.approvals ?? []).filter((r) => decidedStatuses.includes(r.status));
  return (
    <Frame
      id="history-title"
      title={m.historyTitle}
      caption={m.historyCaption}
      loading={requests.isPending}
      failed={requests.isError}
    >
      <AppFilter />
      <RequestTable requests={decided} />
    </Frame>
  );
}
