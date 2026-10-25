// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, Menu } from "@plux/ui";
import { useRouter } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { notifyMessages } from "./notify.messages.ts";

export const docsUrl = "https://docs.plux.dev";
export const onboardingUrl = "https://docs.plux.dev/getting-started";

/** E11: documentation, the shortcut sheet (/me/shortcuts) and onboarding. */
export function HelpMenu(): ReactElement {
  const m = notifyMessages(useLingui().i18n);
  const router = useRouter();
  const open = (url: string) => window.open(url, "_blank", "noopener,noreferrer");
  return (
    <Menu
      trigger={
        <button type="button" aria-label={m.help} className="inline-flex items-center">
          <Icon name="info" size="capsule" />
        </button>
      }
      entries={[
        { kind: "item", id: "docs", label: m.helpDocs, onSelect: () => open(docsUrl) },
        { kind: "item", id: "shortcuts", label: m.helpShortcuts, onSelect: () => router.history.push("/me/shortcuts") },
        { kind: "item", id: "onboarding", label: m.helpOnboarding, onSelect: () => open(onboardingUrl) },
      ]}
    />
  );
}
