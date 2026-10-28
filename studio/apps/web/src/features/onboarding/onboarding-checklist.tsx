// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { useMutation, useQuery } from "@connectrpc/connect-query";
import { useLingui } from "@lingui/react";
import { AppService, PreferencesSchema, PreferencesService } from "@plux/api";
import { Button, Meter, StatusPill } from "@plux/ui";
import { useQuery as useQueryRq } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { onboardingMessages } from "./onboarding.messages.ts";
import { SampleApps } from "./sample-apps.tsx";

/** The steps in the order the specification lists them (STU-015); the server may add more, which follow. */
const order = ["create_app", "pair_device", "edit_screen", "publish_release", "invite_member"] as const;

type Messages = ReturnType<typeof onboardingMessages>;

function stepText(m: Messages, key: string): { readonly label: string; readonly hint: string } | undefined {
  switch (key) {
    case "create_app":
      return { label: m.stepCreate, hint: m.stepCreateHint };
    case "pair_device":
      return { label: m.stepPair, hint: m.stepPairHint };
    case "edit_screen":
      return { label: m.stepEdit, hint: m.stepEditHint };
    case "publish_release":
      return { label: m.stepPublish, hint: m.stepPublishHint };
    case "invite_member":
      return { label: m.stepInvite, hint: m.stepInviteHint };
    default:
      return undefined;
  }
}

/**
 * O1: the first-run checklist (STU-015): create or import an app, pair a device, edit, publish. Progress is derived
 * by the server from what the organisation has done, so a step cannot be ticked by hand; the checklist can be
 * dismissed (stored in the person's preferences) and shown again. It is hidden once every step is done.
 */
export function OnboardingChecklist({
  org,
  canCreate,
  onCreate,
}: {
  readonly org: string;
  readonly canCreate: boolean;
  readonly onCreate: () => void;
}): ReactElement | null {
  const m = onboardingMessages(useLingui().i18n);
  const { session } = useApp();
  const router = useRouter();
  const prefs = useQuery(PreferencesService.method.getPreferences, {});
  const onboarding = useQuery(PreferencesService.method.getOnboarding, {});
  const update = useMutation(PreferencesService.method.updatePreferences);
  const first = useQueryRq({
    queryKey: ["apps", organizationIdOf(org), "first"],
    queryFn: () =>
      createClient(AppService, session.transport).listApps({
        organizationId: organizationIdOf(org),
        page: { pageSize: 1, pageToken: "" },
      }),
  });
  const [failed, setFailed] = useState(false);
  const steps = [...(onboarding.data?.steps ?? [])].sort((a, b) => rank(a.key) - rank(b.key));
  const done = steps.filter((s) => s.done).length;
  if (steps.length === 0 || prefs.data === undefined || done === steps.length) {
    return null;
  }
  const dismissed = prefs.data.preferences?.onboardingDismissed === true;
  const setDismissed = (value: boolean) =>
    update.mutate(
      {
        preferences: create(PreferencesSchema, { onboardingDismissed: value }),
        updateMask: { paths: ["onboarding_dismissed"] },
      },
      { onSuccess: () => void prefs.refetch(), onError: () => setFailed(true) },
    );
  if (dismissed) {
    return (
      <div className="flex items-center gap-2">
        <Button variant="secondary" onClick={() => setDismissed(false)}>
          {m.reopen}
        </Button>
        {failed && (
          <p role="alert" className="text-body text-danger">
            {m.saveFailed}
          </p>
        )}
      </div>
    );
  }
  const app = first.data?.apps[0];
  return (
    <section aria-labelledby="onboarding-title" className="flex flex-col gap-3 rounded-xl bg-card p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="onboarding-title" className="font-serif text-title text-primary">
          {m.title}
        </h2>
        <Meter label={m.progressLabel} value={done} max={steps.length} valueText={m.progressText(done, steps.length)} />
      </div>
      <p className="text-body text-secondary">{m.caption}</p>
      <ol aria-label={m.stepsLabel} className="flex flex-col gap-2">
        {steps.map((step) => {
          const text = stepText(m, step.key);
          return text === undefined ? null : (
            <li key={step.key} className="flex flex-wrap items-center gap-3">
              <StatusPill status={step.done ? "success" : "neutral"} label={step.done ? m.statusDone : m.statusTodo} />
              <div className="flex flex-col">
                <span className="text-body text-primary">{text.label}</span>
                <span className="text-caption text-secondary">{text.hint}</span>
              </div>
              {step.key === "create_app" && !step.done && canCreate && (
                <Button variant="secondary" onClick={onCreate}>
                  {m.newApp}
                </Button>
              )}
              {step.key !== "create_app" && !step.done && app !== undefined && (
                <Link to="/o/$org/apps/$app" params={{ org, app: app.id }} className="text-body">
                  {m.continueIn(app.name)}
                </Link>
              )}
            </li>
          );
        })}
      </ol>
      {canCreate && (
        <SampleApps
          organizationId={organizationIdOf(org)}
          onImported={(id) => router.history.push(`/o/${org}/apps/${id}`)}
        />
      )}
      <div>
        <Button variant="ghost" onClick={() => setDismissed(true)}>
          {m.dismiss}
        </Button>
        {failed && (
          <p role="alert" className="text-body text-danger">
            {m.saveFailed}
          </p>
        )}
      </div>
    </section>
  );
}

function rank(key: string): number {
  const index = (order as readonly string[]).indexOf(key);
  return index === -1 ? order.length : index;
}
