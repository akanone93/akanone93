// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { IdentityService } from "@plux/api";
import { Button, Field, Input } from "@plux/ui";
import { useForm } from "@tanstack/react-form";
import { useParams, useRouter } from "@tanstack/react-router";
import { useState } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { authMessages } from "./auth.messages.ts";
import { inviteSchema } from "./auth.schema.ts";

function firstMessage(errors: readonly unknown[]): string | undefined {
  const first = errors[0];
  return typeof first === "object" && first !== null && "message" in first ? String(first.message) : undefined;
}

/** P4: accept an invitation by choosing a name and a password; the person then signs in. */
export function InvitePage() {
  const { i18n } = useLingui();
  const m = authMessages(i18n);
  const { session } = useApp();
  const router = useRouter();
  const { token } = useParams({ strict: false }) as { token: string };
  const [failed, setFailed] = useState(false);
  const form = useForm({
    defaultValues: { displayName: "", password: "" },
    validators: { onChange: inviteSchema(m) },
    onSubmit: async ({ value }) => {
      setFailed(false);
      try {
        await createClient(IdentityService, session.transport).acceptInvitation({ invitation: token, ...value });
        router.history.push("/signin");
      } catch {
        setFailed(true);
      }
    },
  });
  return (
    <section aria-labelledby="invite-title" className="flex flex-col gap-6">
      <h1 id="invite-title" className="text-screen">
        {m.inviteTitle}
      </h1>
      <form
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          void form.handleSubmit();
        }}
      >
        <form.Field name="displayName">
          {(field) => (
            <Field label={m.displayName} error={firstMessage(field.state.meta.errors)}>
              <Input
                value={field.state.value}
                onChange={(e) => field.handleChange(e.target.value)}
                onBlur={field.handleBlur}
              />
            </Field>
          )}
        </form.Field>
        <form.Field name="password">
          {(field) => (
            <Field label={m.newPassword} error={firstMessage(field.state.meta.errors)}>
              <Input
                type="password"
                autoComplete="new-password"
                value={field.state.value}
                onChange={(e) => field.handleChange(e.target.value)}
                onBlur={field.handleBlur}
              />
            </Field>
          )}
        </form.Field>
        {failed && (
          <p role="alert" className="text-body text-danger">
            {m.inviteFailed}
          </p>
        )}
        <Button type="submit" variant="primary">
          {m.accept}
        </Button>
      </form>
    </section>
  );
}
