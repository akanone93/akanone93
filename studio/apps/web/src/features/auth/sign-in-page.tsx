// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { FederationService, IdentityService, type MfaChallenge, type SignInProvider } from "@plux/api";
import { Button, Field, Input } from "@plux/ui";
import { useLingui } from "@lingui/react";
import { useForm } from "@tanstack/react-form";
import { useQuery } from "@tanstack/react-query";
import { useRouter, useSearch } from "@tanstack/react-router";
import { type ReactNode, useMemo, useState } from "react";
import { forgetUser, safeNext } from "../../lib/guards.ts";
import type { Session } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { authMessages } from "./auth.messages.ts";
import { codeSchema, signInSchema } from "./auth.schema.ts";
import { getAssertion } from "./webauthn.ts";

type Messages = ReturnType<typeof authMessages>;

function messagesOf(errors: readonly unknown[]): string | undefined {
  const first = errors[0];
  return typeof first === "object" && first !== null && "message" in first ? String(first.message) : undefined;
}

function FormError({ children }: { children: ReactNode }) {
  return (
    <p role="alert" className="text-body text-danger">
      {children}
    </p>
  );
}

/** P1: password, then the second factor the server asks for (P2), and the single sign-on buttons (P3). */
export function SignInPage() {
  const { i18n } = useLingui();
  const m = authMessages(i18n);
  const { session } = useApp();
  const router = useRouter();
  const search = useSearch({ strict: false }) as { next?: string };
  const identity = useMemo(() => createClient(IdentityService, session.transport), [session]);
  const [challenge, setChallenge] = useState<MfaChallenge | null>(null);
  const [failure, setFailure] = useState<string | null>(null);

  const finish = (csrfToken: string) => {
    session.setCsrfToken(csrfToken);
    forgetUser(session);
    router.history.push(safeNext(search.next));
  };

  const passwordForm = useForm({
    defaultValues: { email: "", password: "" },
    validators: { onChange: signInSchema(m) },
    onSubmit: async ({ value }) => {
      setFailure(null);
      try {
        const reply = await identity.startPasswordLogin(value);
        if (reply.challenge !== undefined) {
          setChallenge(reply.challenge);
        } else {
          finish(reply.session?.csrfToken ?? "");
        }
      } catch (error) {
        setFailure(error instanceof ConnectError && error.code === Code.Unauthenticated ? m.rejected : m.failed);
      }
    },
  });

  if (challenge !== null) {
    return (
      <FactorStep
        m={m}
        challenge={challenge}
        failure={failure}
        setFailure={setFailure}
        onCode={async (code) => {
          try {
            finish((await identity.completeMfa({ challengeId: challenge.id, code })).session?.csrfToken ?? "");
          } catch {
            setFailure(m.codeRejected);
          }
        }}
        onPasskey={async () => {
          try {
            const options = await identity.beginWebAuthnLogin({ challengeId: challenge.id });
            const assertion = await getAssertion(options.optionsJson, challenge.id);
            finish((await identity.completeWebAuthnLogin(assertion)).session?.csrfToken ?? "");
          } catch {
            setFailure(m.passkeyFailed);
          }
        }}
      />
    );
  }

  return (
    <section aria-labelledby="signin-title" className="flex flex-col gap-6">
      <h1 id="signin-title" className="text-screen">
        {m.signInTitle}
      </h1>
      <form
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          void passwordForm.handleSubmit();
        }}
      >
        <passwordForm.Field name="email">
          {(field) => (
            <Field label={m.email} error={messagesOf(field.state.meta.errors)}>
              <Input
                type="email"
                autoComplete="username"
                value={field.state.value}
                onChange={(e) => field.handleChange(e.target.value)}
                onBlur={field.handleBlur}
              />
            </Field>
          )}
        </passwordForm.Field>
        <passwordForm.Field name="password">
          {(field) => (
            <Field label={m.password} error={messagesOf(field.state.meta.errors)}>
              <Input
                type="password"
                autoComplete="current-password"
                value={field.state.value}
                onChange={(e) => field.handleChange(e.target.value)}
                onBlur={field.handleBlur}
              />
            </Field>
          )}
        </passwordForm.Field>
        {failure !== null && <FormError>{failure}</FormError>}
        <passwordForm.Subscribe selector={(s) => s.isSubmitting}>
          {(submitting) => (
            <Button type="submit" variant="primary" loading={submitting} loadingLabel={m.submitting}>
              {m.submit}
            </Button>
          )}
        </passwordForm.Subscribe>
      </form>
      <Providers m={m} />
    </section>
  );
}

function FactorStep(props: {
  m: Messages;
  challenge: MfaChallenge;
  failure: string | null;
  setFailure: (value: string | null) => void;
  onCode: (code: string) => Promise<void>;
  onPasskey: () => Promise<void>;
}) {
  const { m, challenge } = props;
  const form = useForm({
    defaultValues: { code: "" },
    validators: { onChange: codeSchema(m) },
    onSubmit: async ({ value }) => {
      props.setFailure(null);
      await props.onCode(value.code);
    },
  });
  return (
    <section aria-labelledby="factor-title" className="flex flex-col gap-6">
      <h1 id="factor-title" className="text-screen">
        {m.factorTitle}
      </h1>
      {challenge.kinds.includes("totp") && (
        <form
          noValidate
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            void form.handleSubmit();
          }}
        >
          <form.Field name="code">
            {(field) => (
              <Field label={m.code} error={messagesOf(field.state.meta.errors)}>
                <Input
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  value={field.state.value}
                  onChange={(e) => field.handleChange(e.target.value)}
                  onBlur={field.handleBlur}
                />
              </Field>
            )}
          </form.Field>
          <Button type="submit" variant="primary">
            {m.verify}
          </Button>
        </form>
      )}
      {challenge.kinds.includes("webauthn") && (
        <Button type="button" variant="secondary" onClick={() => void props.onPasskey()}>
          {m.passkey}
        </Button>
      )}
      {props.failure !== null && <FormError>{props.failure}</FormError>}
    </section>
  );
}

/** Starts OIDC: the state goes to the BFF first, as JSON, then the browser leaves for the provider (SEC-101). SAML goes straight to the identity provider. */
async function startProvider(session: Session, provider: SignInProvider): Promise<void> {
  if (provider.kind === "saml") {
    window.location.assign((await createClient(FederationService, session.transport).startSamlLogin({ providerId: provider.id })).url);
    return;
  }
  const started = await createClient(IdentityService, session.transport).startOidcLogin({ providerId: provider.id });
  await fetch("/auth/oidc/state", {
    method: "POST",
    headers: { "content-type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ state: started.state }),
  });
  window.location.assign(started.url);
}

function Providers({ m }: { m: Messages }) {
  const { session } = useApp();
  const providers = useQuery({
    queryKey: ["sign-in-providers"],
    queryFn: () => createClient(FederationService, session.transport).listSignInProviders({}),
  });
  const list = providers.data?.providers ?? [];
  if (list.length === 0) {
    return null;
  }
  return (
    <div role="group" aria-label={m.providers} className="flex flex-col gap-2">
      {list.map((p) => (
        <Button key={p.id} type="button" variant="secondary" onClick={() => void startProvider(session, p)}>
          {m.continueWith(p.name)}
        </Button>
      ))}
    </div>
  );
}
