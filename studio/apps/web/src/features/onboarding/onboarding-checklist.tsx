// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService } from "@plux/api";
import { Button, Icon, Meter } from "@plux/ui";
import { Badge } from "@plux/ui/components/badge";
import { Button as LinkButton } from "@plux/ui/components/button";
import { Item, ItemActions, ItemContent, ItemDescription, ItemMedia, ItemTitle } from "@plux/ui/components/item";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { organizationIdOf } from "../../lib/session.ts";
import { useApp } from "../../lib/use-app.tsx";
import { onboardingMessages } from "./onboarding.messages.ts";
import { SampleApps } from "./sample-apps.tsx";
import { useOnboarding } from "./use-onboarding.ts";

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
 * O1: the first-run checklist (STU-015) as the body of the header's Get started dialog: create or import an app, pair a
 * device, edit, publish, invite. Progress is derived by the server from what the organisation has done, so a step
 * cannot be ticked by hand; the checklist can be dismissed (stored in the person's preferences) and shown again from
 * the help menu.
 */
export function OnboardingChecklist({
  org,
  canCreate,
  onCreate,
  onImported,
  onDismissed,
}: {
  readonly org: string;
  readonly canCreate: boolean;
  readonly onCreate: () => void;
  readonly onImported: (appId: string) => void;
  readonly onDismissed: () => void;
}): ReactElement {
  const m = onboardingMessages(useLingui().i18n);
  const { session } = useApp();
  const onboarding = useOnboarding();
  const first = useQuery({
    queryKey: ["apps", organizationIdOf(org), "first"],
    queryFn: () =>
      createClient(AppService, session.transport).listApps({
        organizationId: organizationIdOf(org),
        page: { pageSize: 1, pageToken: "" },
      }),
  });
  const [failed, setFailed] = useState(false);
  const { steps, done } = onboarding;
  const app = first.data?.apps[0];
  return (
    <section aria-label={m.title} className="flex min-h-0 flex-col gap-6">
      <Meter label={m.progressLabel} value={done} max={steps.length} valueText={m.progressText(done, steps.length)} />
      <ol aria-label={m.stepsLabel} className="flex flex-col gap-2">
        {steps.map((step) => (
          <StepItem key={step.key} step={step} m={m} org={org} app={app} onCreate={canCreate ? onCreate : undefined} />
        ))}
      </ol>
      {canCreate && <SampleApps organizationId={organizationIdOf(org)} onImported={onImported} />}
      <div className="flex flex-wrap items-center justify-between gap-2 border-t pt-4">
        <p className="text-sm text-muted-foreground">{m.dismissHint}</p>
        <Button
          variant="ghost"
          size="sm"
          onClick={async () => {
            if (await onboarding.setDismissed(true)) {
              onDismissed();
            } else {
              setFailed(true);
            }
          }}
        >
          {m.dismiss}
        </Button>
        {failed && (
          <p role="alert" className="text-sm text-destructive">
            {m.saveFailed}
          </p>
        )}
      </div>
    </section>
  );
}
