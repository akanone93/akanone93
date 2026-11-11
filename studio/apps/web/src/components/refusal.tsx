// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { IdentityService } from "@plux/api";
import { Icon, type IconName } from "@plux/ui";
import { Button, buttonVariants } from "@plux/ui/components/button";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@plux/ui/components/empty";
import { Spinner } from "@plux/ui/components/spinner";
import { useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { assertionFields, requestOptions } from "../features/security/step-up.ts";
import { useApp } from "../lib/use-app.tsx";
import { refusalMessages } from "./refusal.messages.ts";

/** Why the server would not answer a page's read: each kind has its own way past it. */
export type RefusalKind = "step-up" | "second-factor" | "licence" | "permission" | "failed";

/** The first of the reads' errors, for a page that loads several. */
export function firstError(...errors: readonly unknown[]): unknown {
  return errors.find((e) => e !== null && e !== undefined);
}

/**
 * The server's own words for a refused call, its Plux code first (SRV-006), or empty for anything else (a network
 * failure, a bug). A failed action shows it under its own message, so the way past the refusal is visible.
 */
export function reasonOf(error: unknown): string {
  return error instanceof ConnectError && /^PLX-\d{4}/.test(error.rawMessage) ? error.rawMessage : "";
}

/** The stable Plux code at the start of a refusal's message (SRV-006), or empty. */
export function plxCode(error: unknown): string {
  return error instanceof ConnectError ? (/PLX-\d{4}/.exec(error.rawMessage)?.[0] ?? "") : "";
}

/**
 * The kind of a failed read, from its Plux code (Appendix F): PLX-6200 step-up required or PLX-6201 too old, PLX-8011 second factor
 * required, PLX-8600 enterprise feature not licensed; any other permission refusal is a missing permission.
 */
export function refusalOf(error: unknown): RefusalKind {
  switch (plxCode(error)) {
    case "PLX-6200":
    case "PLX-6201":
      return "step-up";
    case "PLX-8011":
      return "second-factor";
    case "PLX-8600":
      return "licence";
    default:
      return error instanceof ConnectError && error.code === Code.PermissionDenied ? "permission" : "failed";
  }
}

const icon: Readonly<Record<RefusalKind, IconName>> = {
  "step-up": "key",
  "second-factor": "shield",
  licence: "lock",
  permission: "lock",
  failed: "warning-circle",
};

/**
 * A page the server refused, said plainly with the way past it: a passkey confirmation for a step-up (SEC-103), the
 * licence page for an enterprise feature, the administrator for a permission, or another try. After a confirmation or
 * a retry the page's reads run again.
 */
export function Refusal({
  error,
  failedTitle,
}: {
  readonly error: unknown;
  /** The area's own words for a read that failed for no reason the server named. */
  readonly failedTitle?: string;
}): ReactElement {
  const m = refusalMessages(useLingui().i18n);
  const kind = refusalOf(error);
  const code = plxCode(error);
  const queryClient = useQueryClient();
  const { session } = useApp();
  const org = (useParams({ strict: false }) as { org?: string }).org;
  const [busy, setBusy] = useState(false);
  const [stepUpFailed, setStepUpFailed] = useState(false);
  const again = () => queryClient.refetchQueries({ type: "active" });

  const confirm = async () => {
    setBusy(true);
    setStepUpFailed(false);
    try {
      const identity = createClient(IdentityService, session.transport);
      const { requestId, optionsJson } = await identity.beginStepUp({});
      const credential = await navigator.credentials.get(requestOptions(optionsJson));
      if (credential === null) {
        throw new Error("cancelled");
      }
      await identity.completeStepUp({ requestId, ...assertionFields(credential) });
      await again();
    } catch {
      setStepUpFailed(true);
    } finally {
      setBusy(false);
    }
  };

  const words = {
    "step-up": [m.stepUpTitle, m.stepUpBody],
    "second-factor": [m.secondFactorTitle, m.secondFactorBody],
    licence: [m.licenceTitle, m.licenceBody],
    permission: [m.permissionTitle, m.permissionBody],
    failed: [failedTitle ?? m.failedTitle, m.failedBody],
  }[kind];

  return (
    <Empty role="alert" className="refusal border">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Icon name={icon[kind]} />
        </EmptyMedia>
        <EmptyTitle>{words[0]}</EmptyTitle>
        <EmptyDescription>{words[1]}</EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        {kind === "step-up" && (
          <Button onClick={() => void confirm()} disabled={busy}>
            {busy ? <Spinner data-icon="inline-start" /> : <Icon name="key" data-icon="inline-start" />}
            {m.stepUpAction}
          </Button>
        )}
        {stepUpFailed && <p className="text-sm text-destructive">{m.stepUpFailed}</p>}
        {kind === "licence" && org !== undefined && (
          <Link to="/o/$org/admin/license" params={{ org }} className={buttonVariants({ variant: "default" })}>
            {m.licenceAction}
          </Link>
        )}
        {kind === "failed" && (
          <Button variant="outline" onClick={() => void again()}>
            <Icon name="arrow-clockwise" data-icon="inline-start" />
            {m.retry}
          </Button>
        )}
        {code !== "" && <p className="text-xs text-muted-foreground tabular-nums">{m.code(code)}</p>}
      </EmptyContent>
    </Empty>
  );
}
