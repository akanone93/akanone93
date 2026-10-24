// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { GovernanceService } from "@plux/api";
import { Button } from "@plux/ui";
import { useParams } from "@tanstack/react-router";
import { useState } from "react";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { chromeMessages } from "./shell-chrome.messages.ts";

function Message({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="flex flex-col gap-3">
      <h1 className="text-screen">{title}</h1>
      {children}
    </section>
  );
}

export function NotFoundPage() {
  const m = chromeMessages(useLingui().i18n);
  return (
    <Message title={m.notFoundTitle}>
      <p className="text-body text-secondary">{m.notFoundBody}</p>
    </Message>
  );
}

/** The in-shell 403 page; "Explain access" asks GovernanceService.ExplainAccess why (F14, GOV-005). */
export function ForbiddenPage({ permission }: { permission: string }) {
  const m = chromeMessages(useLingui().i18n);
  const { session } = useApp();
  const org = (useParams({ strict: false }) as { org?: string }).org;
  const [verdict, setVerdict] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const explain = async () => {
    setFailed(false);
    try {
      const reply = await createClient(GovernanceService, session.transport).explainAccess({
        organizationId: org === undefined ? "" : organizationIdOf(org),
        permission,
      });
      setVerdict(reply.decision);
    } catch {
      setFailed(true);
    }
  };
  return (
    <Message title={m.forbiddenTitle}>
      <p className="text-body text-secondary">{m.forbiddenBody}</p>
      <div>
        <Button type="button" variant="secondary" onClick={() => void explain()}>
          {m.explain}
        </Button>
      </div>
      {verdict !== null && <p role="status">{m.decision(verdict)}</p>}
      {failed && (
        <p role="alert" className="text-danger">
          {m.explainFailed}
        </p>
      )}
    </Message>
  );
}

/** Shown to administrators for an enterprise-only page without a licence (R1). */
export function LicencePage() {
  const m = chromeMessages(useLingui().i18n);
  return (
    <Message title={m.licenceTitle}>
      <p className="text-body text-secondary">{m.licenceBody}</p>
    </Message>
  );
}
