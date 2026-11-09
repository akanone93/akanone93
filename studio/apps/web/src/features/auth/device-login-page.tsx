// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useQuery } from "@connectrpc/connect-query";
import { useLingui } from "@lingui/react";
import { IdentityService, OrgService } from "@plux/api";
import { Icon, Select } from "@plux/ui";
import { Alert, AlertDescription } from "@plux/ui/components/alert";
import { Button } from "@plux/ui/components/button";
import { Card, CardContent } from "@plux/ui/components/card";
import { Checkbox } from "@plux/ui/components/checkbox";
import { Field, FieldLabel } from "@plux/ui/components/field";
import { Input } from "@plux/ui/components/input";
import { useSearch } from "@tanstack/react-router";
import { type ReactElement, useId, useState } from "react";
import { PageHeader } from "../../components/page.tsx";
import { plxCode } from "../../components/refusal.tsx";
import { useApp } from "../../lib/use-app.tsx";
import { useCurrentUser } from "../../lib/use-current-user.ts";
import { authMessages } from "./auth.messages.ts";

type Outcome = "idle" | "approved" | "denied" | "needs-factor" | "not-found" | "failed";

/**
 * The user code of RFC 8628 as typed or carried in the link: the server's alphabet is consonants, in two groups of
 * four, so anything else is dropped and the length is bounded before it reaches the field or the server.
 */
export function cleanUserCode(raw: string | undefined): string {
  return (raw ?? "")
    .toUpperCase()
    .replace(/[^A-Z-]/g, "")
    .slice(0, 9);
}

function outcomeOf(error: unknown): Outcome {
  switch (plxCode(error)) {
    case "PLX-8011":
      return "needs-factor";
    case "PLX-8031":
      return "not-found";
    default:
      return "failed";
  }
}

/**
 * Where a person approves `plux login` (CLI-002, RFC 8628): the code from the terminal, the organisation the command
 * line will act in, and an explicit confirmation that the person started this sign-in, because a link with someone
 * else's code is the device grant's phishing path. Nothing is approved on arrival. The server checks the rest: a
 * browser session that presented a second factor, and a token that can do no more than its approver.
 */
export function DeviceLoginPage(): ReactElement {
  const m = authMessages(useLingui().i18n);
  const userCode = (useSearch({ strict: false }) as { user_code?: string }).user_code;
  const { session } = useApp();
  const user = useCurrentUser().data;
  const orgs = useQuery(OrgService.method.listOrganizations, {});
  const member = new Set(user?.memberships.map((x) => x.organizationId));
  const organisations = (orgs.data?.organizations ?? []).filter((o) => member.has(o.id));
  const [code, setCode] = useState(() => cleanUserCode(userCode));
  const [chosen, setChosen] = useState<string | undefined>(undefined);
  const [mine, setMine] = useState(false);
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<Outcome>("idle");
  const codeId = useId();
  const mineId = useId();
  const home = session.home();
  const organizationId = chosen ?? (organisations.some((o) => o.id === home) ? home : organisations[0]?.id) ?? "";
  const done = outcome === "approved" || outcome === "denied";
  const complete = /^[A-Z]{4}-?[A-Z]{4}$/.test(code);

  const decide = async (approve: boolean) => {
    setBusy(true);
    setOutcome("idle");
    const identity = createClient(IdentityService, session.transport);
    try {
      if (approve) {
        await identity.approveDeviceAuthorization({ userCode: code, organizationId });
      } else {
        await identity.denyDeviceAuthorization({ userCode: code });
      }
      setOutcome(approve ? "approved" : "denied");
    } catch (e) {
      setOutcome(outcomeOf(e));
    } finally {
      setBusy(false);
    }
  };

  const message: Record<Exclude<Outcome, "idle">, string> = {
    approved: m.deviceApproved,
    denied: m.deviceDenied,
    "needs-factor": m.deviceNeedsFactor,
    "not-found": m.deviceNotFound,
    failed: m.deviceFailed,
  };
  return (
    <section className="page-frame" aria-labelledby="device-title">
      <PageHeader id="device-title" title={m.deviceTitle} caption={m.deviceCaption} icon="terminal" />
      <div className="page-body">
        <Card className="max-w-xl">
          <CardContent className="flex flex-col gap-4">
            <Alert>
              <Icon name="warning" />
              <AlertDescription>{m.deviceWarning}</AlertDescription>
            </Alert>
            <Field>
              <FieldLabel htmlFor={codeId}>{m.deviceCode}</FieldLabel>
              <Input
                id={codeId}
                value={code}
                maxLength={9}
                autoComplete="off"
                spellCheck={false}
                className="font-mono text-lg tracking-widest"
                disabled={done}
                onChange={(e) => {
                  setCode(cleanUserCode(e.target.value));
                  setMine(false);
                }}
              />
            </Field>
            <Select
              label={m.deviceOrganisation}
              value={organizationId}
              disabled={done || organisations.length === 0}
              options={organisations.map((o) => ({ value: o.id, label: o.name }))}
              onValueChange={(v) => setChosen(v)}
            />
            <Field orientation="horizontal">
              <Checkbox id={mineId} checked={mine} disabled={done} onCheckedChange={(v) => setMine(v === true)} />
              <FieldLabel htmlFor={mineId}>{m.deviceMine}</FieldLabel>
            </Field>
            <div className="flex flex-wrap gap-2">
              <Button
                onClick={() => void decide(true)}
                disabled={busy || done || !mine || !complete || organizationId === ""}
              >
                {m.deviceApprove}
              </Button>
              <Button variant="outline" onClick={() => void decide(false)} disabled={busy || done || !complete}>
                {m.deviceDeny}
              </Button>
            </div>
            {outcome !== "idle" && (
              <p role={done ? "status" : "alert"} className="text-sm">
                {message[outcome]}
              </p>
            )}
          </CardContent>
        </Card>
      </div>
    </section>
  );
}
