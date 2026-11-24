// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { useLingui } from "@lingui/react";
import { type UserIdentityProvider, UserIdentityProviderSchema } from "@plux/api";
import { Field, Input, Textarea } from "@plux/ui";
import { type ReactElement, useEffect, useState } from "react";
import { gatewayMessages } from "./gateway.messages.ts";
import { ChangeButton, GatewayFrame, linesOf, RemoveDialog, useGatewayScope } from "./parts.tsx";
import { useGatewayChanges, useIdentityProvider } from "./use-gateway.ts";

interface ProviderDraft {
  readonly issuer: string;
  readonly jwksUrl: string;
  readonly audiences: string;
  readonly claims: string;
}

const emptyProvider: ProviderDraft = { issuer: "", jwksUrl: "https://", audiences: "", claims: "" };

function draftOf(p: UserIdentityProvider | null | undefined): ProviderDraft {
  return p === null || p === undefined
    ? emptyProvider
    : {
        issuer: p.issuer,
        jwksUrl: p.jwksUrl,
        audiences: p.audiences.join("\n"),
        claims: p.forwardClaims.join("\n"),
      };
}

/** The provider a draft describes for an environment; undefined while the issuer, keys or an audience is missing. */
export function providerOf(d: ProviderDraft, appId: string, environmentId: string): UserIdentityProvider | undefined {
  const audiences = linesOf(d.audiences);
  if (d.issuer.trim() === "" || d.jwksUrl.trim() === "" || audiences.length === 0) {
    return undefined;
  }
  return create(UserIdentityProviderSchema, {
    appId,
    environmentId,
    issuer: d.issuer.trim(),
    jwksUrl: d.jwksUrl.trim(),
    audiences,
    forwardClaims: linesOf(d.claims),
  });
}

/** The identity provider whose user tokens an environment's devices may exchange (SEC-026, build log row 199). */
export function IdentityProviderPage(): ReactElement {
  const m = gatewayMessages(useLingui().i18n);
  const { app, environment, can } = useGatewayScope();
  const environmentId = environment.environment?.id;
  const provider = useIdentityProvider(app, environmentId);
  const changes = useGatewayChanges();
  const [d, setD] = useState<ProviderDraft>(emptyProvider);
  const [tried, setTried] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [saveError, setSaveError] = useState<unknown>(undefined);
  // The form starts from what the environment trusts, and again when it changes.
  useEffect(() => setD(draftOf(provider.data)), [provider.data]);
  const set = (change: Partial<ProviderDraft>) => setD({ ...d, ...change });
  const run = (work: Promise<unknown>) => {
    setSaveError(undefined);
    work.catch((e: unknown) => setSaveError(e));
  };
  const save = () => {
    const made = environmentId === undefined ? undefined : providerOf(d, app, environmentId);
    if (made === undefined) {
      setTried(true);
      return;
    }
    run(changes.saveProvider(made));
  };
  const missing = (empty: boolean) => (tried && empty ? { error: m.required } : {});
  return (
    <GatewayFrame
      id="gateway-identity-title"
      title={m.identityTitle}
      caption={m.identityCaption}
      loading={environment.loading || provider.isPending}
      failed={environment.failed || provider.isError}
      error={provider.error}
      saveError={saveError}
    >
      {provider.data === null && <p className="text-body text-muted-foreground">{m.noProvider}</p>}
      <div className="flex max-w-xl flex-col gap-4">
        <Field label={m.fieldIssuer} {...missing(d.issuer.trim() === "")}>
          <Input dir="ltr" disabled={!can} value={d.issuer} onChange={(e) => set({ issuer: e.target.value })} />
        </Field>
        <Field label={m.fieldJwks} {...missing(d.jwksUrl.trim() === "")}>
          <Input dir="ltr" disabled={!can} value={d.jwksUrl} onChange={(e) => set({ jwksUrl: e.target.value })} />
        </Field>
        <Field label={m.fieldAudiences} {...missing(linesOf(d.audiences).length === 0)}>
          <Textarea
            dir="ltr"
            disabled={!can}
            value={d.audiences}
            onChange={(e) => set({ audiences: e.target.value })}
          />
        </Field>
        <Field label={m.fieldClaims}>
          <Textarea dir="ltr" disabled={!can} value={d.claims} onChange={(e) => set({ claims: e.target.value })} />
        </Field>
        <div className="flex gap-2">
          <ChangeButton can={can} label={m.saveProvider} onClick={save} />
          {provider.data !== null && provider.data !== undefined && (
            <ChangeButton can={can} variant="secondary" label={m.removeProvider} onClick={() => setRemoving(true)} />
          )}
        </div>
      </div>
      <RemoveDialog
        name={removing ? (provider.data?.issuer ?? "") : undefined}
        m={m}
        onClose={() => setRemoving(false)}
        onConfirm={() => {
          setRemoving(false);
          if (environmentId !== undefined) {
            run(changes.deleteProvider(app, environmentId));
          }
        }}
      />
    </GatewayFrame>
  );
}
