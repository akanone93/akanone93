// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { GovernanceService } from "@plux/api";
import { Button } from "@plux/ui";
import { buttonVariants } from "@plux/ui/components/button";
import { Link, useParams, useRouter } from "@tanstack/react-router";
import { useState } from "react";
import { PageEmpty, PageHeader } from "../../components/page.tsx";
import { plxCode, withReason } from "../../components/refusal.tsx";
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

/**
 * An address no page answers: inside the app it shows in the shell, outside it on its own; either way with the way to
 * the home page.
 */
export function NotFoundPage() {
  const m = chromeMessages(useLingui().i18n);
  return (
    <section aria-labelledby="not-found-title" className="page-frame">
      <PageHeader id="not-found-title" icon="question" title={m.notFoundTitle} caption={m.notFoundBody} />
      <div className="page-body">
        <PageEmpty
          icon="question"
          text={m.notFoundHint}
          action={
            <Link to="/" className={buttonVariants({ size: "sm" })}>
              {m.notFoundHome}
            </Link>
          }
        />
      </div>
    </section>
  );
}

/** The in-shell 403 page; "Explain access" asks GovernanceService.ExplainAccess why (F14, GOV-005). */
export function ForbiddenPage({ permission }: { permission: string }) {
  const m = chromeMessages(useLingui().i18n);
  const { session } = useApp();
  const org = (useParams({ strict: false }) as { org?: string }).org;
  const [verdict, setVerdict] = useState<string | null>(null);
  const [failed, setFailed] = useState<unknown>(null);
  const explain = async () => {
    setFailed(null);
    try {
      const reply = await createClient(GovernanceService, session.transport).explainAccess({
        organizationId: org === undefined ? "" : organizationIdOf(org),
        permission,
      });
      setVerdict(reply.decision);
    } catch (error) {
      setFailed(error);
    }
  };
  return (
    <Message title={m.forbiddenTitle}>
      <p className="text-body text-muted-foreground">{m.forbiddenBody}</p>
      <div>
        <Button type="button" variant="secondary" onClick={() => void explain()}>
          {m.explain}
        </Button>
      </div>
      {verdict !== null && <p role="status">{m.decision(verdict)}</p>}
      {failed !== null && (
        <p role="alert" className="text-danger">
          {withReason(m.explainFailed, failed)}
        </p>
      )}
    </Message>
  );
}

/**
 * A page whose loading failed for a reason other than access: the server unreachable, or any other failure. It says
 * which, gives the server's code when there is one (never the message or a stack, which may hold internals), and loads
 * the page again on request.
 */
export function FailurePage({ error, reset }: { readonly error: unknown; readonly reset: () => void }) {
  const m = chromeMessages(useLingui().i18n);
  const router = useRouter();
  const unreachable =
    error instanceof ConnectError && (error.code === Code.Unavailable || error.code === Code.DeadlineExceeded);
  const code = plxCode(error);
  const again = async () => {
    await router.invalidate();
    reset();
  };
  return (
    <section aria-labelledby="failure-title" className="page-frame" role="alert">
      <PageHeader
        id="failure-title"
        icon="warning-circle"
        title={unreachable ? m.unreachableTitle : m.failedTitle}
        caption={unreachable ? m.unreachableBody : m.failedBody}
      />
      <div className="page-body flex flex-col items-start gap-3">
        <Button type="button" size="sm" onClick={() => void again()}>
          {m.tryAgain}
        </Button>
        {code !== "" && <p className="text-xs text-muted-foreground tabular-nums">{m.errorCode(code)}</p>}
      </div>
    </section>
  );
}

/** Shown to administrators for an enterprise-only page without a licence (R1). */
export function LicencePage() {
  const m = chromeMessages(useLingui().i18n);
  return (
    <Message title={m.licenceTitle}>
      <p className="text-body text-muted-foreground">{m.licenceBody}</p>
    </Message>
  );
}
