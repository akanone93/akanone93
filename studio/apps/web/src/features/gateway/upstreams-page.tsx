// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create, fromJson, type JsonValue, toJson } from "@bufbuild/protobuf";
import { useLingui } from "@lingui/react";
import {
  type Upstream,
  UpstreamApiKeySchema,
  UpstreamMtlsSchema,
  UpstreamOAuthClientCredentialsSchema,
  UpstreamOperationSchema,
  UpstreamSchema,
} from "@plux/api";
import { Dialog, Field, Input, Select, Textarea } from "@plux/ui";
import { Checkbox } from "@plux/ui/components/checkbox";
import { type ReactElement, useId, useState } from "react";
import { DataTable } from "../../components/page.tsx";
import { type GatewayMessages, gatewayMessages } from "./gateway.messages.ts";
import { ChangeButton, GatewayFrame, linesOf, RemoveDialog, useGatewayScope } from "./parts.tsx";
import { useGatewayChanges, useUpstreams } from "./use-gateway.ts";

const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE"] as const;
const LEVELS = ["AL0", "AL1", "AL2", "AL3"] as const;
type AuthKind = "none" | "apiKey" | "oauth" | "mtls";

/** An upstream as the dialog edits it: every field as text, the operations as their JSON list. */
export interface UpstreamDraft {
  readonly id: string;
  readonly name: string;
  readonly baseUrl: string;
  readonly auth: AuthKind;
  readonly header: string;
  readonly secretRef: string;
  readonly tokenUrl: string;
  readonly clientId: string;
  readonly scopes: string;
  readonly methods: readonly string[];
  readonly assurance: string;
  readonly timeoutMs: string;
  readonly audience: string;
  readonly allowPrivate: boolean;
  readonly operations: string;
}

export const emptyDraft: UpstreamDraft = {
  id: "",
  name: "",
  baseUrl: "https://",
  auth: "none",
  header: "",
  secretRef: "",
  tokenUrl: "",
  clientId: "",
  scopes: "",
  methods: ["GET"],
  assurance: "AL0",
  timeoutMs: "",
  audience: "",
  allowPrivate: false,
  operations: "",
};

/** The sign-in fields of a stored upstream's `auth`. */
function authDraftOf(
  a: Upstream["auth"],
): Pick<UpstreamDraft, "auth" | "header" | "secretRef" | "tokenUrl" | "clientId" | "scopes"> {
  switch (a.case) {
    case "apiKey":
      return { ...emptyDraft, auth: "apiKey", header: a.value.header, secretRef: a.value.secretRef };
    case "oauthClientCredentials":
      return {
        ...emptyDraft,
        auth: "oauth",
        secretRef: a.value.clientSecretRef,
        tokenUrl: a.value.tokenUrl,
        clientId: a.value.clientId,
        scopes: a.value.scopes.join("\n"),
      };
    case "mtls":
      return { ...emptyDraft, auth: "mtls", secretRef: a.value.clientCertSecretRef };
    default:
      return emptyDraft;
  }
}

/** The draft of a stored upstream. */
export function draftOf(u: Upstream): UpstreamDraft {
  const operations = u.operations.map((o) => toJson(UpstreamOperationSchema, o));
  return {
    ...emptyDraft,
    ...authDraftOf(u.auth),
    id: u.id,
    name: u.name,
    baseUrl: u.baseUrl,
    methods: u.allowedMethods,
    assurance: u.requiresAssurance || "AL0",
    timeoutMs: u.timeoutMs === 0 ? "" : String(u.timeoutMs),
    audience: u.audience,
    allowPrivate: u.allowPrivateAddresses,
    operations: operations.length === 0 ? "" : JSON.stringify(operations, null, 2),
  };
}

/** How the gateway signs in, as the message's `auth` case. */
function authOf(d: UpstreamDraft): Upstream["auth"] {
  switch (d.auth) {
    case "apiKey":
      return { case: "apiKey", value: create(UpstreamApiKeySchema, { header: d.header, secretRef: d.secretRef }) };
    case "oauth":
      return {
        case: "oauthClientCredentials",
        value: create(UpstreamOAuthClientCredentialsSchema, {
          tokenUrl: d.tokenUrl,
          clientId: d.clientId,
          clientSecretRef: d.secretRef,
          scopes: linesOf(d.scopes),
        }),
      };
    case "mtls":
      return { case: "mtls", value: create(UpstreamMtlsSchema, { clientCertSecretRef: d.secretRef }) };
    default:
      return { case: undefined };
  }
}

/** The upstream a draft describes for an environment, or the reason it cannot be one yet. */
export function upstreamOf(
  d: UpstreamDraft,
  appId: string,
  environmentId: string,
): { readonly upstream: Upstream } | { readonly error: "required" | "operations" } {
  if (d.name.trim() === "" || d.baseUrl.trim() === "" || d.methods.length === 0) {
    return { error: "required" };
  }
  let operations: Upstream["operations"] = [];
  if (d.operations.trim() !== "") {
    try {
      const list: unknown = JSON.parse(d.operations);
      if (!Array.isArray(list)) {
        return { error: "operations" };
      }
      operations = list.map((o) => fromJson(UpstreamOperationSchema, o as JsonValue));
    } catch {
      return { error: "operations" };
    }
  }
  const timeout = Number.parseInt(d.timeoutMs, 10);
  return {
    upstream: create(UpstreamSchema, {
      id: d.id,
      appId,
      environmentId,
      name: d.name.trim(),
      baseUrl: d.baseUrl.trim(),
      auth: authOf(d),
      allowPrivateAddresses: d.allowPrivate,
      timeoutMs: Number.isFinite(timeout) && timeout > 0 ? timeout : 0,
      allowedMethods: [...d.methods],
      requiresAssurance: d.assurance === "AL0" ? "" : d.assurance,
      operations,
      audience: d.audience.trim(),
    }),
  };
}

/** The gateway's upstreams of one environment (SEC-030), each editable and removable (build log row 199). */
export function UpstreamsPage(): ReactElement {
  const m = gatewayMessages(useLingui().i18n);
  const { app, environment, can } = useGatewayScope();
  const environmentId = environment.environment?.id;
  const list = useUpstreams(app, environmentId);
  const changes = useGatewayChanges();
  const [draft, setDraft] = useState<UpstreamDraft | undefined>(undefined);
  const [removing, setRemoving] = useState<Upstream | undefined>(undefined);
  const [saveError, setSaveError] = useState<unknown>(undefined);
  const authName: Record<string, string> = {
    apiKey: m.authApiKey,
    oauthClientCredentials: m.authOAuth,
    mtls: m.authMtls,
  };
  const run = (work: Promise<unknown>) => {
    setSaveError(undefined);
    work.catch((e: unknown) => setSaveError(e));
  };
  const upstreams = list.data?.upstreams ?? [];
  return (
    <GatewayFrame
      id="gateway-upstreams-title"
      title={m.upstreamsTitle}
      caption={m.upstreamsCaption}
      loading={environment.loading || list.isPending}
      failed={environment.failed || list.isError}
      error={list.error}
      saveError={saveError}
    >
      <div>
        <ChangeButton can={can} label={m.addUpstream} onClick={() => setDraft(emptyDraft)} />
      </div>
      <DataTable
        caption={m.upstreamsTitle}
        heads={[m.colName, m.colBaseUrl, m.colAuth, m.colMethods, m.colOperations, m.colActions]}
        actions
        empty={m.noUpstreams}
        rows={upstreams.map((u) => ({
          key: u.id,
          cells: [
            <span key="n" className="font-mono">
              {u.name}
            </span>,
            <span key="u" className="font-mono" dir="ltr">
              {u.baseUrl}
            </span>,
            authName[u.auth.case ?? ""] ?? m.authNone,
            u.allowedMethods.join(", "),
            String(u.operations.length),
            <span key="a" className="inline-flex gap-1">
              <ChangeButton can={can} size="sm" variant="ghost" label={m.edit} onClick={() => setDraft(draftOf(u))} />
              <ChangeButton can={can} size="sm" variant="ghost" label={m.remove} onClick={() => setRemoving(u)} />
            </span>,
          ],
        }))}
      />
      {draft !== undefined && environmentId !== undefined && (
        <UpstreamDialog
          draft={draft}
          m={m}
          onClose={() => setDraft(undefined)}
          onSave={(u) => {
            setDraft(undefined);
            run(changes.saveUpstream(u));
          }}
          app={app}
          environmentId={environmentId}
        />
      )}
      <RemoveDialog
        name={removing?.name}
        m={m}
        onClose={() => setRemoving(undefined)}
        onConfirm={() => {
          const id = removing?.id;
          setRemoving(undefined);
          if (id !== undefined) {
            run(changes.deleteUpstream(id));
          }
        }}
      />
    </GatewayFrame>
  );
}

/** The fields of the way the gateway signs in, by kind. */
function AuthFields(props: {
  readonly d: UpstreamDraft;
  readonly set: (change: Partial<UpstreamDraft>) => void;
  readonly m: GatewayMessages;
}): ReactElement | null {
  const { d, set, m } = props;
  if (d.auth === "none") {
    return null;
  }
  return (
    <>
      {d.auth === "apiKey" && (
        <Field label={m.fieldHeader}>
          <Input value={d.header} onChange={(e) => set({ header: e.target.value })} />
        </Field>
      )}
      {d.auth === "oauth" && (
        <>
          <Field label={m.fieldTokenUrl}>
            <Input dir="ltr" value={d.tokenUrl} onChange={(e) => set({ tokenUrl: e.target.value })} />
          </Field>
          <Field label={m.fieldClientId}>
            <Input value={d.clientId} onChange={(e) => set({ clientId: e.target.value })} />
          </Field>
          <Field label={m.fieldScopes}>
            <Textarea value={d.scopes} onChange={(e) => set({ scopes: e.target.value })} />
          </Field>
        </>
      )}
      <Field label={m.fieldSecretRef}>
        <Input value={d.secretRef} onChange={(e) => set({ secretRef: e.target.value })} />
      </Field>
    </>
  );
}

/** The dialog that adds or edits an upstream. */
function UpstreamDialog(props: {
  readonly draft: UpstreamDraft;
  readonly m: GatewayMessages;
  readonly app: string;
  readonly environmentId: string;
  readonly onClose: () => void;
  readonly onSave: (upstream: Upstream) => void;
}): ReactElement {
  const { m } = props;
  const [d, setD] = useState(props.draft);
  const [error, setError] = useState<string | undefined>(undefined);
  const privateId = useId();
  const set = (change: Partial<UpstreamDraft>) => setD({ ...d, ...change });
  const save = () => {
    const made = upstreamOf(d, props.app, props.environmentId);
    if ("error" in made) {
      setError(made.error === "required" ? m.required : m.operationsInvalid);
      return;
    }
    props.onSave(made.upstream);
  };
  const toggle = (method: string) =>
    set({ methods: d.methods.includes(method) ? d.methods.filter((x) => x !== method) : [...d.methods, method] });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) {
          props.onClose();
        }
      }}
      title={m.upstreamDialog}
      description={m.upstreamDialogDescription}
      labels={{ close: m.close }}
    >
      <Field label={m.fieldName} {...(error === m.required && d.name.trim() === "" ? { error } : {})}>
        <Input value={d.name} onChange={(e) => set({ name: e.target.value })} />
      </Field>
      <Field label={m.fieldBaseUrl}>
        <Input dir="ltr" value={d.baseUrl} onChange={(e) => set({ baseUrl: e.target.value })} />
      </Field>
      <Field label={m.fieldAuth}>
        <Select
          label={m.fieldAuth}
          options={[
            { value: "none", label: m.authNone },
            { value: "apiKey", label: m.authApiKey },
            { value: "oauth", label: m.authOAuth },
            { value: "mtls", label: m.authMtls },
          ]}
          value={d.auth}
          onValueChange={(v) => set({ auth: v as AuthKind })}
        />
      </Field>
      <AuthFields d={d} set={set} m={m} />
      <fieldset className="flex flex-col gap-2">
        <legend className="text-label">{m.fieldMethods}</legend>
        <div className="flex flex-wrap gap-3">
          {METHODS.map((method) => (
            <span key={method} className="flex items-center gap-1.5 font-mono text-data">
              <Checkbox
                id={`${privateId}-${method}`}
                checked={d.methods.includes(method)}
                onCheckedChange={() => toggle(method)}
              />
              <label htmlFor={`${privateId}-${method}`}>{method}</label>
            </span>
          ))}
        </div>
      </fieldset>
      <Field label={m.fieldAssurance}>
        <Select
          label={m.fieldAssurance}
          options={LEVELS.map((l) => ({ value: l, label: l }))}
          value={d.assurance}
          onValueChange={(v) => set({ assurance: v })}
        />
      </Field>
      <Field label={m.fieldTimeout}>
        <Input inputMode="numeric" value={d.timeoutMs} onChange={(e) => set({ timeoutMs: e.target.value })} />
      </Field>
      <Field label={m.fieldAudience}>
        <Input dir="ltr" value={d.audience} onChange={(e) => set({ audience: e.target.value })} />
      </Field>
      <div className="flex flex-col gap-1">
        <div className="flex items-center gap-2 text-body">
          <Checkbox
            id={privateId}
            checked={d.allowPrivate}
            onCheckedChange={(v) => set({ allowPrivate: v === true })}
          />
          <label htmlFor={privateId}>{m.fieldPrivate}</label>
        </div>
        {d.allowPrivate && (
          <p role="alert" className="text-caption text-danger">
            {m.privateWarning}
          </p>
        )}
      </div>
      <Field label={m.fieldOperations} {...(error === m.operationsInvalid ? { error } : {})}>
        <Textarea
          spellCheck={false}
          dir="ltr"
          rows={6}
          value={d.operations}
          onChange={(e) => set({ operations: e.target.value })}
        />
      </Field>
      {error === m.required && (
        <p role="alert" className="text-caption text-danger">
          {m.required}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <ChangeButton can label={m.cancel} variant="secondary" onClick={props.onClose} />
        <ChangeButton can label={m.save} onClick={save} />
      </div>
    </Dialog>
  );
}
