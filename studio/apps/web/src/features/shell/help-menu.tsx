// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, Menu } from "@plux/ui";
import { Button } from "@plux/ui/components/button";
import { useRouter } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { onboardingMessages } from "../onboarding/onboarding.messages.ts";
import { notifyMessages } from "./notify.messages.ts";

/**
 * E11: documentation and the quick-start guide (only with a configured docs site), the dismissed getting-started
 * checklist, and the shortcut sheet (/me/shortcuts).
 */
export function HelpMenu({
  docsUrl,
  onShowGettingStarted,
}: {
  readonly docsUrl: string | undefined;
  /** Shows the dismissed getting-started checklist again (STU-015); undefined while there is none to show. */
  readonly onShowGettingStarted?: (() => void) | undefined;
}): ReactElement {
  const { i18n } = useLingui();
  const m = notifyMessages(i18n);
  const reopen = onboardingMessages(i18n).reopen;
  const router = useRouter();
  const open = (url: string) => window.open(url, "_blank", "noopener,noreferrer");
  return (
    <Menu
      trigger={
        <Button variant="ghost" size="icon-sm" aria-label={m.help}>
          <Icon name="question" />
        </Button>
      }
      entries={[
        ...(docsUrl === undefined
          ? []
          : [{ kind: "item" as const, id: "docs", label: m.helpDocs, onSelect: () => open(docsUrl) }]),
        ...(onShowGettingStarted === undefined
          ? []
          : [{ kind: "item" as const, id: "get-started", label: reopen, onSelect: onShowGettingStarted }]),
        { kind: "item", id: "shortcuts", label: m.helpShortcuts, onSelect: () => router.history.push("/me/shortcuts") },
        ...(docsUrl === undefined
          ? []
          : [
              {
                kind: "item" as const,
                id: "onboarding",
                label: m.helpOnboarding,
                onSelect: () => open(`${docsUrl}/guides/quick-start`),
              },
            ]),
      ]}
    />
  );
}
