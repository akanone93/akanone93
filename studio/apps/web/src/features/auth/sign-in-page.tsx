// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { FederationService, IdentityService, type MfaChallenge, type SignInProvider } from "@plux/api";
import { Button, Field, Icon, type IconName, Input } from "@plux/ui";
import { useForm, useStore } from "@tanstack/react-form";
import { useQuery } from "@tanstack/react-query";
import { useRouter, useSearch } from "@tanstack/react-router";
import { domAnimation, LazyMotion } from "motion/react";
import { type ReactNode, useMemo, useState } from "react";
import { shownError } from "../../lib/field-error.ts";
import { forgetUser, safeNext } from "../../lib/guards.ts";
import type { Session } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { Mark, Rise, RisingWords } from "../shell/brand.tsx";
import { authMessages } from "./auth.messages.ts";
import { codeSchema, signInSchema } from "./auth.schema.ts";
import { ScreenRing } from "./screen-ring.tsx";
import "./sign-in.css";
import { getAssertion } from "./webauthn.ts";

type Messages = ReturnType<typeof authMessages>;

function FormError({ children }: { children: ReactNode }) {
  return (
    <p role="alert" className="text-body text-danger">
      {children}
    </p>
  );
}

/** One of the short facts in the footer of the left half. */
function Fact({ icon, children }: { icon: IconName; children: ReactNode }) {
  return (
    <li className="flex items-center gap-2 text-caption text-tertiary">
      <Icon name={icon} size="tab" />
      {children}
    </li>
  );
}

/**
 * The sign-in page (U16.2): on a white page, the left half holds the mark, the promise, the form
 * and a quiet footer of facts; on the right, sample apps built with Plux turn on a ring.
 */
function SignInFrame({ children }: { children: ReactNode }) {
  const { i18n } = useLingui();
  const m = authMessages(i18n);
  const lineOne = m.heroLineOne;
  return (
    <LazyMotion features={domAnimation} strict>
      <div className="grid min-h-screen bg-raised lg:grid-cols-12">
        <div className="flex flex-col px-6 py-8 sm:px-12 lg:col-span-6 lg:px-16 xl:px-24">
          <header className="flex h-10 shrink-0 items-center gap-3">
            <Mark />
            <span className="font-wordmark text-section font-semibold">{m.wordmark}</span>
          </header>
          <main className="signin-main flex flex-1 flex-col justify-center py-12">
            <div className="flex w-full max-w-dialog-md flex-col">
              <h1 className="signin-title text-display-sm lg:text-display">
                <RisingWords text={lineOne} from={0} className="block" />
                <RisingWords text={m.heroLineTwo} from={lineOne.split(" ").length} className="block text-tertiary" />
              </h1>
              <Rise delay={0.5} className="mt-5">
                <p className="max-w-auth-form text-lead text-muted-foreground">{m.heroLead}</p>
              </Rise>
              <Rise delay={0.7} className="signin-form mt-12 w-full max-w-auth-form">
                {children}
              </Rise>
            </div>
          </main>
          <Rise delay={0.9}>
            <footer>
              <ul className="flex flex-wrap gap-x-6 gap-y-2">
                <Fact icon="key">{m.factSigned}</Fact>
                <Fact icon="lightning">{m.factInstant}</Fact>
                <Fact icon="check-circle">{m.factNative}</Fact>
              </ul>
            </footer>
          </Rise>
        </div>
        <div className="relative hidden overflow-hidden lg:col-span-6 lg:block">
          <ScreenRing />
        </div>
      </div>
    </LazyMotion>
  );
}

/** The password field's button that shows or hides what was typed. */
function RevealButton({ shown, onToggle, m }: { shown: boolean; onToggle: () => void; m: Messages }) {
  return (
    <button
      type="button"
      aria-label={shown ? m.hidePassword : m.showPassword}
      aria-pressed={shown}
      onClick={onToggle}
      className="grid size-8 place-items-center rounded-md text-tertiary transition duration-fast hover:bg-hover hover:text-foreground focus-visible:outline-2 focus-visible:outline-focus"
    >
      <Icon name={shown ? "eye-slash" : "eye"} size="tab" />
    </button>
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
      <section aria-labelledby="signin-title" className="flex flex-col gap-5">
        <h2 id="signin-title" className="text-section font-semibold">
          {m.signInTitle}
        </h2>
        <form
          noValidate
          className="flex flex-col gap-5"
          onSubmit={(event) => {
            event.preventDefault();
            void passwordForm.handleSubmit();
          }}
        >
          <passwordForm.Field name="email">
            {(field) => (
              <Field label={m.email} error={shownError(field.state.meta, submitted)}>
                <Input
                  size="lg"
                  leading="user"
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
                  size="lg"
                  leading="lock"
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
              <Button type="submit" variant="primary" size="lg" loading={submitting} loadingLabel={m.submitting}>
                {m.submit}
              </Button>
            )}
          </passwordForm.Subscribe>
        </form>
        <Providers m={m} />
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
      <h2 id="factor-title" className="text-section font-semibold">
        {m.factorTitle}
      </h2>
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
                  size="lg"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  value={field.state.value}
                  onChange={(e) => field.handleChange(e.target.value)}
                  onBlur={field.handleBlur}
                />
              </Field>
            )}
          </form.Field>
          <Button type="submit" variant="primary" size="lg">
            {m.verify}
          </Button>
        </form>
      )}
      {challenge.kinds.includes("webauthn") && (
        <Button type="button" variant="secondary" size="lg" onClick={() => void props.onPasskey()}>
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
    <fieldset className="flex flex-col gap-2">
      <legend className="sr-only">{m.providers}</legend>
      {list.map((p) => (
        <Button key={p.id} type="button" variant="secondary" size="lg" onClick={() => void startProvider(session, p)}>
          {m.continueWith(p.name)}
        </Button>
      ))}
    </fieldset>
  );
}
