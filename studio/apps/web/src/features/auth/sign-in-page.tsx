// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { FederationService, IdentityService, type MfaChallenge, type SignInProvider } from "@plux/api";
import { Button, Field, Icon, type IconName, Input } from "@plux/ui";
import { Alert, AlertTitle } from "@plux/ui/components/alert";
import { FieldGroup, FieldSeparator } from "@plux/ui/components/field";
import { InputGroupButton } from "@plux/ui/components/input-group";
import { useForm, useStore } from "@tanstack/react-form";
import { useQuery } from "@tanstack/react-query";
import { useRouter, useSearch } from "@tanstack/react-router";
import { type ReactNode, useMemo, useState } from "react";
import { shownError } from "../../lib/field-error.ts";
import { forgetUser, safeNext } from "../../lib/guards.ts";
import type { Session } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { authMessages } from "./auth.messages.ts";
import { codeSchema, signInSchema } from "./auth.schema.ts";
import { ScreenRing } from "./screen-ring.tsx";
import { getAssertion } from "./webauthn.ts";

type Messages = ReturnType<typeof authMessages>;

function FormError({ children }: { children: ReactNode }) {
  return (
    <Alert variant="destructive">
      <Icon name="warning-circle" />
      <AlertTitle>{children}</AlertTitle>
    </Alert>
  );
}

/** One of the short facts at the foot of the form column. */
function Fact({ icon, children }: { icon: IconName; children: ReactNode }) {
  return (
    <li className="flex items-center gap-1.5">
      <Icon name={icon} size="pill" />
      {children}
    </li>
  );
}

/**
 * The sign-in page: shadcn's login-02 block. The form column holds the product's name, the form and a quiet row of
 * facts; the other half shows sample apps built with Plux turning on a ring (U16.2), unchanged.
 */
function SignInFrame({ children }: { children: ReactNode }) {
  const m = authMessages(useLingui().i18n);
  return (
    <div className="grid min-h-svh lg:grid-cols-2">
      <div className="flex flex-col gap-4 p-6 md:p-10">
        <div className="flex justify-center gap-2 md:justify-start">
          <span className="flex items-center gap-2 font-medium">
            <span className="flex size-6 items-center justify-center rounded-md bg-primary text-primary-foreground">
              <Icon name="stack" size="pill" />
            </span>
            {m.wordmark}
          </span>
        </div>
        <main className="flex flex-1 items-center justify-center">
          <div className="w-full max-w-xs">{children}</div>
        </main>
        <ul className="flex flex-wrap justify-center gap-x-4 gap-y-1 text-xs text-muted-foreground md:justify-start">
          <Fact icon="key">{m.factSigned}</Fact>
          <Fact icon="lightning">{m.factInstant}</Fact>
          <Fact icon="check-circle">{m.factNative}</Fact>
        </ul>
      </div>
      <div className="relative hidden overflow-hidden bg-muted lg:block">
        <ScreenRing />
      </div>
    </div>
  );
}

/** The password field's button that shows or hides what was typed. */
function RevealButton({ shown, onToggle, m }: { shown: boolean; onToggle: () => void; m: Messages }) {
  return (
    <InputGroupButton
      size="icon-xs"
      aria-label={shown ? m.hidePassword : m.showPassword}
      aria-pressed={shown}
      onClick={onToggle}
    >
      <Icon name={shown ? "eye-slash" : "eye"} />
    </InputGroupButton>
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
  const [reveal, setReveal] = useState(false);

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
  const submitted = useStore(passwordForm.store, (state) => state.submissionAttempts > 0);

  if (challenge !== null) {
    return (
      <SignInFrame>
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
      </SignInFrame>
    );
  }

  return (
    <SignInFrame>
      <section aria-labelledby="signin-title" className="flex flex-col gap-6">
        <form
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            void passwordForm.handleSubmit();
          }}
        >
          <FieldGroup>
            <div className="flex flex-col items-center gap-1 text-center">
              <h1 id="signin-title" className="text-2xl font-bold">
                {m.signInTitle}
              </h1>
              <p className="text-sm text-balance text-muted-foreground">{m.heroLead}</p>
            </div>
            <passwordForm.Field name="email">
              {(field) => (
                <Field label={m.email} error={shownError(field.state.meta, submitted)}>
                  <Input
                    placeholder={m.emailPlaceholder}
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
                <Field label={m.password} error={shownError(field.state.meta, submitted)}>
                  <Input
                    trailing={<RevealButton shown={reveal} onToggle={() => setReveal((v) => !v)} m={m} />}
                    type={reveal ? "text" : "password"}
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
                <Button type="submit" className="w-full" loading={submitting} loadingLabel={m.submitting}>
                  {m.submit}
                </Button>
              )}
            </passwordForm.Subscribe>
            <Providers m={m} />
          </FieldGroup>
        </form>
      </section>
    </SignInFrame>
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
  const submitted = useStore(form.store, (state) => state.submissionAttempts > 0);
  return (
    <section aria-labelledby="factor-title" className="flex flex-col gap-6">
      <h1 id="factor-title" className="text-center text-2xl font-bold">
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
              <Field label={m.code} error={shownError(field.state.meta, submitted)}>
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
          <Button type="submit" className="w-full">
            {m.verify}
          </Button>
        </form>
      )}
      {challenge.kinds.includes("webauthn") && (
        <Button type="button" variant="secondary" className="w-full" onClick={() => void props.onPasskey()}>
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
    window.location.assign(
      (await createClient(FederationService, session.transport).startSamlLogin({ providerId: provider.id })).url,
    );
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
    <>
      <FieldSeparator>{m.orContinue}</FieldSeparator>
      <fieldset className="m-0 flex flex-col gap-2 border-0 p-0">
        <legend className="sr-only">{m.providers}</legend>
        {list.map((p) => (
          <Button
            key={p.id}
            type="button"
            variant="secondary"
            className="w-full"
            onClick={() => void startProvider(session, p)}
          >
            {m.continueWith(p.name)}
          </Button>
        ))}
      </fieldset>
    </>
  );
}
