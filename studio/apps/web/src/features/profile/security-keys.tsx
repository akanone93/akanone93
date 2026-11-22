// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { createClient } from "@connectrpc/connect";
import { useQuery } from "@connectrpc/connect-query";
import { useLingui } from "@lingui/react";
import { type Factor, IdentityService } from "@plux/api";
import { AlertDialog, Icon } from "@plux/ui";
import { Badge } from "@plux/ui/components/badge";
import { Button } from "@plux/ui/components/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@plux/ui/components/card";
import { Field, FieldError, FieldLabel } from "@plux/ui/components/field";
import { Input } from "@plux/ui/components/input";
import { Spinner } from "@plux/ui/components/spinner";
import { type ReactElement, useId, useState } from "react";
import { plxCode } from "../../components/refusal.tsx";
import { useApp } from "../../lib/use-app.tsx";
import { createCredential, webAuthnAvailable } from "../auth/webauthn.ts";
import { profileMessages } from "./profile.messages.ts";

type Messages = ReturnType<typeof profileMessages>;
type Status = "idle" | "added" | "failed" | "needs-factor" | "remove-failed";

/** The words for what a refused call means; PLX-8011 asks for a second factor presented in this session (SEC-100). */
function refusedStatus(error: unknown, otherwise: Status): Status {
  return plxCode(error) === "PLX-8011" ? "needs-factor" : otherwise;
}

/**
 * The name field and button that add a key. The name is required (#232): the server refuses a key without one, and
 * the field says so before anything is sent. One addition runs at a time.
 */
function AddKeyForm({
  m,
  busy,
  onAdd,
}: {
  readonly m: Messages;
  readonly busy: boolean;
  readonly onAdd: (label: string) => Promise<boolean>;
}): ReactElement {
  const [label, setLabel] = useState("");
  const [touched, setTouched] = useState(false);
  const labelId = useId();
  const named = label.trim() !== "";
  const invalid = touched && !named;
  const submit = async () => {
    setTouched(true);
    if (!named || busy) {
      return;
    }
    if (await onAdd(label.trim())) {
      setLabel("");
      setTouched(false);
    }
  };
  return (
    <div className="flex flex-wrap items-end gap-3">
      <Field className="min-w-48 flex-1" data-invalid={invalid}>
        <FieldLabel htmlFor={labelId}>{m.keyLabel}</FieldLabel>
        <Input
          id={labelId}
          value={label}
          required
          maxLength={100}
          autoComplete="off"
          aria-invalid={invalid}
          aria-describedby={invalid ? `${labelId}-error` : undefined}
          onChange={(e) => setLabel(e.target.value)}
          onBlur={() => setTouched(true)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              void submit();
            }
          }}
        />
        {invalid && <FieldError id={`${labelId}-error`}>{m.keyNameRequired}</FieldError>}
      </Field>
      <Button onClick={() => void submit()} disabled={busy || !named}>
        {busy ? <Spinner data-icon="inline-start" /> : <Icon name="key" data-icon="inline-start" />}
        {busy ? m.keyAdding : m.keyAdd}
      </Button>
    </div>
  );
}

/**
 * The person's security keys and passkeys (SEC-100, SEC-103): add one with the browser's WebAuthn ceremony, see when
 * each was last used, and remove one after a confirmation. The server decides every rule: a key can be added to an
 * account that has one, or removed, only in a session that presented a second factor.
 */
export function SecurityKeysCard(): ReactElement {
  const { i18n } = useLingui();
  const m = profileMessages(i18n);
  const { session } = useApp();
  const factors = useQuery(IdentityService.method.listFactors, {});
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState<Status>("idle");
  const [removing, setRemoving] = useState<Factor | null>(null);
  const supported = webAuthnAvailable();

  /** Registers a key named `label`; true when it was added, so the form can clear its field. */
  const add = async (label: string): Promise<boolean> => {
    setBusy(true);
    setStatus("idle");
    const identity = createClient(IdentityService, session.transport);
    try {
      const begun = await identity.beginWebAuthnRegistration({ label });
      const factorId = begun.factor?.id ?? "";
      const credential = await createCredential(begun.optionsJson);
      await identity.finishWebAuthnRegistration({ factorId, ...credential });
      setStatus("added");
      return true;
    } catch (e) {
      setStatus(refusedStatus(e, "failed"));
      return false;
    } finally {
      setBusy(false);
      await factors.refetch();
    }
  };

  const remove = async (factor: Factor) => {
    setStatus("idle");
    try {
      await createClient(IdentityService, session.transport).deleteFactor({ factorId: factor.id });
    } catch (e) {
      setStatus(refusedStatus(e, "remove-failed"));
    } finally {
      await factors.refetch();
    }
  };

  const message: Record<Exclude<Status, "idle">, string> = {
    added: m.keyAdded,
    failed: m.keyFailed,
    "needs-factor": m.keyNeedsFactor,
    "remove-failed": m.keyRemoveFailed,
  };
  const list = factors.data?.factors ?? [];
  return (
    <Card>
      <CardHeader>
        <CardTitle>{m.keysTitle}</CardTitle>
        <CardDescription>{m.keysCaption}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {factors.isSuccess && list.length === 0 && <p className="text-sm text-muted-foreground">{m.keysEmpty}</p>}
        {list.length > 0 && (
          <ul aria-label={m.keysTitle} className="flex flex-col gap-2">
            {list.map((f) => (
              <FactorRow key={f.id} factor={f} m={m} locale={i18n.locale} onRemove={() => setRemoving(f)} />
            ))}
          </ul>
        )}
        {supported ? (
          <AddKeyForm m={m} busy={busy} onAdd={add} />
        ) : (
          <p role="alert" className="text-sm text-destructive">
            {m.keyUnsupported}
          </p>
        )}
        {status !== "idle" && (
          <p role={status === "added" ? "status" : "alert"} className="text-sm">
            {message[status]}
          </p>
        )}
      </CardContent>
      <AlertDialog
        open={removing !== null}
        onOpenChange={(open) => {
          if (!open) {
            setRemoving(null);
          }
        }}
        title={m.keyRemoveTitle}
        description={m.keyRemoveBody}
        labels={{ cancel: m.keyCancel, confirm: m.keyRemoveConfirm }}
        onConfirm={() => {
          const factor = removing;
          setRemoving(null);
          if (factor !== null) {
            void remove(factor);
          }
        }}
      />
    </Card>
  );
}

function FactorRow(p: {
  readonly factor: Factor;
  readonly m: Messages;
  readonly locale: string;
  readonly onRemove: () => void;
}): ReactElement {
  const { factor: f, m } = p;
  const name = f.label || m.keyUnnamed;
  const lastUsed =
    f.lastUsedAt === undefined
      ? undefined
      : new Intl.DateTimeFormat(p.locale, { dateStyle: "medium", timeStyle: "short" }).format(
          timestampDate(f.lastUsedAt),
        );
  return (
    <li className="flex flex-wrap items-center gap-3 rounded-lg border px-3 py-2">
      <Icon name={f.kind === "totp" ? "device-mobile" : "key"} />
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-sm font-medium">{name}</span>
        <span className="text-xs text-muted-foreground">
          {f.kind === "totp" ? m.keyKindTotp : m.keyKindWebAuthn}
          {lastUsed !== undefined && ` · ${m.keyLastUsed(lastUsed)}`}
        </span>
      </div>
      {!f.confirmed && <Badge variant="outline">{m.keyPending}</Badge>}
      <Button variant="ghost" size="icon" aria-label={m.keyRemove(name)} onClick={p.onRemove}>
        <Icon name="trash" />
      </Button>
    </li>
  );
}
