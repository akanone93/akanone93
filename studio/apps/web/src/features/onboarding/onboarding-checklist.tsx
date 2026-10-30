// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { create } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { useMutation, useQuery } from "@connectrpc/connect-query";
import { useLingui } from "@lingui/react";
import { AppService, PreferencesSchema, PreferencesService } from "@plux/api";
import { Button, Icon, Meter } from "@plux/ui";
import { Badge } from "@plux/ui/components/badge";
import { Button as LinkButton } from "@plux/ui/components/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@plux/ui/components/card";
import { Item, ItemActions, ItemContent, ItemDescription, ItemMedia, ItemTitle } from "@plux/ui/components/item";
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

/** One step of the checklist: shadcn's Item with its state and, while it is open, the action that does it. */
function StepItem({
  step,
  m,
  org,
  app,
  onCreate,
}: {
  readonly step: { readonly key: string; readonly done: boolean };
  readonly m: Messages;
  readonly org: string;
  readonly app: { readonly id: string; readonly name: string } | undefined;
  /** Opens the create dialog; undefined when the person may not create apps. */
  readonly onCreate: (() => void) | undefined;
}): ReactElement | null {
  const text = stepText(m, step.key);
  if (text === undefined) {
    return null;
  }
  const open = !step.done;
  return (
    <Item variant="outline" render={<li />}>
      <ItemMedia>
        <Badge variant={step.done ? "secondary" : "outline"} className="w-20 justify-start">
          <Icon
            name={step.done ? "check-circle" : "clock"}
            data-icon="inline-start"
            className={step.done ? "text-success" : "text-muted-foreground"}
          />
          {step.done ? m.statusDone : m.statusTodo}
        </Badge>
      </ItemMedia>
      <ItemContent>
        <ItemTitle>{text.label}</ItemTitle>
        <ItemDescription>{text.hint}</ItemDescription>
      </ItemContent>
      <ItemActions>
        {open && step.key === "create_app" && onCreate !== undefined && (
          <Button size="sm" onClick={onCreate}>
            {m.newApp}
          </Button>
        )}
        {open && step.key !== "create_app" && app !== undefined && (
          <LinkButton
            variant="outline"
            size="sm"
            render={<Link to="/o/$org/apps/$app" params={{ org, app: app.id }} />}
          >
            {m.continueIn(app.name)}
          </LinkButton>
        )}
      </ItemActions>
    </Item>
  );
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
      <div className="flex items-center gap-2 px-4 lg:px-6">
        <Button variant="secondary" size="sm" onClick={() => setDismissed(false)}>
          <Icon name="rocket-launch" data-icon="inline-start" />
          {m.reopen}
        </Button>
        {failed && (
          <p role="alert" className="text-sm text-destructive">
            {m.saveFailed}
          </p>
        )}
      </div>
    );
  }
  const app = first.data?.apps[0];
  return (
    <section aria-labelledby="onboarding-title" className="px-4 lg:px-6">
      <Card>
        <CardHeader>
          <CardTitle id="onboarding-title" className="text-base">
            {m.title}
          </CardTitle>
          <CardDescription>{m.caption}</CardDescription>
          <CardAction>
            <Meter
              label={m.progressLabel}
              value={done}
              max={steps.length}
              valueText={m.progressText(done, steps.length)}
            />
          </CardAction>
        </CardHeader>
        <CardContent className="grid gap-6 lg:grid-cols-5">
          <ol aria-label={m.stepsLabel} className="flex flex-col gap-2 lg:col-span-3">
            {steps.map((step) => (
              <StepItem
                key={step.key}
                step={step}
                m={m}
                org={org}
                app={app}
                onCreate={canCreate ? onCreate : undefined}
              />
            ))}
          </ol>
          {canCreate && (
            <div className="lg:col-span-2">
              <SampleApps
                organizationId={organizationIdOf(org)}
                onImported={(id) => router.history.push(`/o/${org}/apps/${id}`)}
              />
            </div>
          )}
        </CardContent>
        <CardFooter className="gap-2 border-t">
          <Button variant="ghost" size="sm" onClick={() => setDismissed(true)}>
            {m.dismiss}
          </Button>
          {failed && (
            <p role="alert" className="text-sm text-destructive">
              {m.saveFailed}
            </p>
          )}
        </CardFooter>
      </Card>
    </section>
  );
}

function rank(key: string): number {
  const index = (order as readonly string[]).indexOf(key);
  return index === -1 ? order.length : index;
}
