// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { AuditService } from "@plux/api";
import { Button, Field, Input, Select, Textarea } from "@plux/ui";
import { Checkbox } from "@plux/ui/components/checkbox";
import { FieldLabel, Field as FieldRow } from "@plux/ui/components/field";
import { type FormEvent, type ReactElement, useId, useState } from "react";
import { adminMessages } from "./admin.messages.ts";
import { LicenceNote } from "./parts.tsx";
import { useLicensed, useRpc } from "./use-admin.ts";

/** The kinds of destination: syslog is the core's; the others need the `siem-extended` licence (SEC-143). */
const coreKind = "syslog";
const licensedKinds = ["cef", "otlp", "webhook"] as const;

interface Draft {
  name: string;
  kind: string;
  address: string;
  caPem: string;
  clientCertificatePem: string;
  clientKeyPem: string;
  signingSecret: string;
  allowPrivate: boolean;
}

const empty: Draft = {
  name: "",
  kind: coreKind,
  address: "",
  caPem: "",
  clientCertificatePem: "",
  clientKeyPem: "",
  signingSecret: "",
  allowPrivate: false,
};

/**
 * O27: adds a destination the organisation's audit chain is exported to (SEC-143). Syslog is offered always; CEF, OTLP
 * logs and signed webhooks when the licence includes them. The credentials (client key, signing secret) are sent once,
 * stored sealed by the server and never shown again; the form forgets them after a successful save. A private address
 * is refused unless allowed here, which is for development (SEC-104).
 */
export function DestinationForm({
  can,
  run,
}: {
  readonly can: boolean;
  readonly run: (change: () => Promise<unknown>) => Promise<boolean>;
}): ReactElement {
  const m = adminMessages(useLingui().i18n);
  const audit = useRpc(AuditService);
  const licensed = useLicensed("siem-extended");
  const [draft, setDraft] = useState<Draft>(empty);
  const privateId = useId();
  const kinds = licensed === false ? [coreKind] : [coreKind, ...licensedKinds];
  const set = (field: keyof Draft) => (value: string) => setDraft({ ...draft, [field]: value });
  const secured = draft.kind === "syslog" || draft.kind === "cef";
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const ok = await run(() =>
      audit.createExportDestination({
        name: draft.name,
        kind: draft.kind,
        address: draft.address,
        allowPrivate: draft.allowPrivate,
        caPem: draft.caPem,
        clientCertificatePem: secured ? draft.clientCertificatePem : "",
        clientKeyPem: secured ? draft.clientKeyPem : "",
        signingSecret: draft.kind === "webhook" ? draft.signingSecret : "",
      }),
    );
    if (ok) {
      setDraft(empty);
    }
  };
  return (
    <form aria-label={m.destinationNew} className="flex flex-col gap-3" onSubmit={(e) => void submit(e)}>
      <div className="flex flex-wrap items-end gap-3">
        <Field label={m.colName}>
          <Input required value={draft.name} onChange={(e) => set("name")(e.target.value)} />
        </Field>
        <Field label={m.colKind}>
          <Select
            label={m.colKind}
            options={kinds.map((k) => ({ value: k, label: m.destinationKind(k) }))}
            value={draft.kind}
            onValueChange={set("kind")}
          />
        </Field>
        <Field label={m.destinationAddress}>
          <Input
            required
            value={draft.address}
            placeholder={draft.kind === "syslog" || draft.kind === "cef" ? "siem.example.com:6514" : "https://"}
            onChange={(e) => set("address")(e.target.value)}
          />
        </Field>
      </div>
      {licensed === false && <LicenceNote text={m.destinationsNeedLicence} />}
      <Field label={m.destinationCa}>
        <Textarea rows={3} value={draft.caPem} onChange={(e) => set("caPem")(e.target.value)} />
      </Field>
      {secured && (
        <div className="flex flex-wrap gap-3">
          <Field label={m.destinationClientCertificate}>
            <Textarea
              rows={3}
              value={draft.clientCertificatePem}
              onChange={(e) => set("clientCertificatePem")(e.target.value)}
            />
          </Field>
          <Field label={m.destinationClientKey}>
            <Textarea
              rows={3}
              autoComplete="off"
              spellCheck={false}
              value={draft.clientKeyPem}
              onChange={(e) => set("clientKeyPem")(e.target.value)}
            />
          </Field>
        </div>
      )}
      {draft.kind === "webhook" && (
        <Field label={m.destinationSigningSecret}>
          <Input
            type="password"
            autoComplete="off"
            required
            value={draft.signingSecret}
            onChange={(e) => set("signingSecret")(e.target.value)}
          />
        </Field>
      )}
      <FieldRow orientation="horizontal">
        <Checkbox
          id={privateId}
          checked={draft.allowPrivate}
          onCheckedChange={(v) => setDraft({ ...draft, allowPrivate: v === true })}
        />
        <FieldLabel htmlFor={privateId}>{m.destinationAllowPrivate}</FieldLabel>
      </FieldRow>
      <div>
        <Button type="submit" disabled={!can} title={can ? undefined : m.noPermission}>
          {m.destinationAdd}
        </Button>
      </div>
    </form>
  );
}
