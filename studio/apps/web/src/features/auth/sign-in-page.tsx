// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { FederationService, IdentityService, type MfaChallenge, type SignInProvider } from "@plux/api";
import { Button, Field, Icon, Input } from "@plux/ui";
import { useForm } from "@tanstack/react-form";
import { useQuery } from "@tanstack/react-query";
import { useRouter, useSearch } from "@tanstack/react-router";
import { domAnimation, LazyMotion, m as motion, useReducedMotion } from "motion/react";
import { type ReactNode, useMemo, useState } from "react";
import { forgetUser, safeNext } from "../../lib/guards.ts";
import type { Session } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { authMessages } from "./auth.messages.ts";
import { codeSchema, signInSchema } from "./auth.schema.ts";
import { ScreenRing } from "./screen-ring.tsx";
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

const enter = [0.16, 1, 0.3, 1] as const;

/** Headline words that rise out of a blur, one after another (U13.4). */
function RisingWords({ text, from, className }: { text: string; from: number; className?: string }) {
  const reduced = useReducedMotion() === true;
  return (
    <span className={className}>
      {text.split(" ").map((word, i) => (
        <motion.span
          // biome-ignore lint/suspicious/noArrayIndexKey: the words of a fixed sentence keep their order
          key={i}
          className="inline-block whitespace-pre"
          initial={reduced ? false : { opacity: 0, y: "0.45em", filter: "blur(12px)" }}
          animate={{ opacity: 1, y: 0, filter: "blur(0px)" }}
          transition={{ duration: 1.1, ease: enter, delay: reduced ? 0 : 0.1 + (from + i) * 0.07 }}
        >
          {`${word} `}
        </motion.span>
      ))}
    </span>
  );
}

/** Fades a block up into place after the headline (U13.4). */
function Rise({ delay, children, className }: { delay: number; children: ReactNode; className?: string }) {
  const reduced = useReducedMotion() === true;
  return (
    <motion.div
      className={className}
      initial={reduced ? false : { opacity: 0, y: 24 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 1, ease: enter, delay: reduced ? 0 : delay }}
    >
      {children}
    </motion.div>
  );
}

/** One of the short facts under the form. */
function Fact({ children }: { children: ReactNode }) {
  return (
    <li className="flex items-center gap-2 text-caption text-secondary">
      <Icon name="check-circle" size="tab" />
      {children}
    </li>
  );
}

/**
 * The sign-in page (U16.2): on a white page, the promise, the form and three facts fill the left
 * half; on the right, sample app screens built with Plux turn on a ring and lift their layers.
 */
function SignInFrame({ children }: { children: ReactNode }) {
  const { i18n } = useLingui();
  const m = authMessages(i18n);
  const lineOne = m.heroLineOne;
  return (
    <LazyMotion features={domAnimation} strict>
      <div className="grid min-h-screen bg-raised lg:grid-cols-12">
        <div className="flex flex-col px-6 py-8 sm:px-12 lg:col-span-6 lg:px-16 xl:px-24">
          <header className="flex h-10 shrink-0 items-center">
            <span className="font-wordmark text-section font-semibold">{m.wordmark}</span>
          </header>
          <main className="flex flex-1 flex-col justify-center gap-10 py-12">
            <div className="flex flex-col gap-5">
              <h1 className="text-display-sm lg:text-display">
                <RisingWords text={lineOne} from={0} className="block" />
                <RisingWords text={m.heroLineTwo} from={lineOne.split(" ").length} className="block text-tertiary" />
              </h1>
              <Rise delay={0.5}>
                <p className="max-w-auth-form text-lead text-secondary">{m.heroLead}</p>
              </Rise>
            </div>
            <Rise delay={0.7} className="w-full max-w-auth-form">
              {children}
            </Rise>
            <Rise delay={0.9}>
              <ul className="flex flex-col gap-2">
                <Fact>{m.factSigned}</Fact>
                <Fact>{m.factInstant}</Fact>
                <Fact>{m.factNative}</Fact>
              </ul>
            </Rise>
          </main>
        </div>
        <div className="relative hidden overflow-hidden lg:col-span-6 lg:block">
          <ScreenRing />
        </div>
      </div>
    </LazyMotion>
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
        <h2 id="signin-title" className="text-section font-semibold">
          {m.signInTitle}
        </h2>
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
              <Field
                label={m.email}
                error={field.state.meta.isTouched ? messagesOf(field.state.meta.errors) : undefined}
              >
                <Input
                  size="lg"
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
              <Field
                label={m.password}
                error={field.state.meta.isTouched ? messagesOf(field.state.meta.errors) : undefined}
              >
                <Input
                  size="lg"
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
              <Field
                label={m.code}
                error={field.state.meta.isTouched ? messagesOf(field.state.meta.errors) : undefined}
              >
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
