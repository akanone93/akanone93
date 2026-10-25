// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { GovernanceService } from "@plux/api";
import { Dialog } from "@plux/ui";
import { useQuery } from "@tanstack/react-query";
import type { ReactElement } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { dialogMessages } from "./dialogs.messages.ts";

export interface ExplainAccessDialogProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  readonly organizationId: string;
  readonly permission: string;
}

/** D8 (F14): asks GovernanceService.ExplainAccess why `permission` is denied. */
export function ExplainAccessDialog({
  open,
  onOpenChange,
  organizationId,
  permission,
}: ExplainAccessDialogProps): ReactElement {
  const m = dialogMessages(useLingui().i18n);
  const { session } = useApp();
  const query = useQuery({
    enabled: open,
    queryKey: ["explain-access", organizationId, permission],
    queryFn: () => createClient(GovernanceService, session.transport).explainAccess({ organizationId, permission }),
  });
  return (
    <Dialog open={open} onOpenChange={onOpenChange} size="md" title={m.explainTitle} labels={{ close: m.close }}>
      {query.isPending && <p className="text-body text-secondary">{m.explainLoading}</p>}
      {query.isError && (
        <p role="alert" className="text-body text-danger">
          {m.explainFailed}
        </p>
      )}
      {query.data !== undefined && <p className="font-mono text-data">{m.explainDecision(query.data.decision)}</p>}
    </Dialog>
  );
}
