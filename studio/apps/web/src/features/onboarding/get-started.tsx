// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Dialog, Icon } from "@plux/ui";
import { Badge } from "@plux/ui/components/badge";
import { Button } from "@plux/ui/components/button";
import { Spinner } from "@plux/ui/components/spinner";
import { useRouter } from "@tanstack/react-router";
import { lazy, type ReactElement, Suspense } from "react";
import { commandMessages } from "../../lib/commands/index.ts";
import { onboardingMessages } from "./onboarding.messages.ts";
import { useOnboarding } from "./use-onboarding.ts";

// The checklist and the sample apps load when the dialog first opens, not with every page.
const Checklist = lazy(() =>
  import("./onboarding-checklist.tsx").then((module) => ({ default: module.OnboardingChecklist })),
);

/**
 * O1 in the header (STU-015): while steps remain and the person has not dismissed it, a Get started button with the
 * progress, which opens the checklist in a dialog.
 */
export function GetStarted({
  org,
  canCreate,
  open,
  onOpenChange,
}: {
  /** The `:org` of the page. */
  readonly org: string;
  readonly canCreate: boolean;
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
}): ReactElement | null {
  const { i18n } = useLingui();
  const m = onboardingMessages(i18n);
  const router = useRouter();
  const onboarding = useOnboarding();
  if (!onboarding.ready || onboarding.complete || onboarding.dismissed) {
    return null;
  }
  const go = (path: string) => {
    onOpenChange(false);
    router.history.push(path);
  };
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={m.title}
      description={m.caption}
      size="lg"
      labels={{ close: commandMessages(i18n).close }}
      trigger={
        <Button variant="outline" size="sm" aria-label={m.title}>
          <Icon name="rocket-launch" data-icon="inline-start" />
          <span className="hidden md:inline">{m.title}</span>
          <Badge variant="secondary" className="tabular-nums">
            {m.progressShort(onboarding.done, onboarding.steps.length)}
          </Badge>
        </Button>
      }
    >
      <Suspense fallback={<Spinner />}>
        <Checklist
          org={org}
          canCreate={canCreate}
          onCreate={() => go(`/o/${org}/apps/new`)}
          onImported={(id) => go(`/o/${org}/apps/${id}`)}
          onDismissed={() => onOpenChange(false)}
        />
      </Suspense>
    </Dialog>
  );
}
