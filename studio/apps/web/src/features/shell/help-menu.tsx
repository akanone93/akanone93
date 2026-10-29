// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, Menu } from "@plux/ui";
import { useRouter } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { notifyMessages } from "./notify.messages.ts";

/** E11: documentation and getting started (only with a configured docs site), and the shortcut sheet (/me/shortcuts). */
export function HelpMenu({ docsUrl }: { readonly docsUrl: string | undefined }): ReactElement {
  const m = notifyMessages(useLingui().i18n);
  const router = useRouter();
  const open = (url: string) => window.open(url, "_blank", "noopener,noreferrer");
  return (
    <Menu
      trigger={
        <button type="button" aria-label={m.help} className="inline-flex items-center">
          <Icon name="question" size="capsule" />
        </button>
      }
      entries={[
        ...(docsUrl === undefined
          ? []
          : [{ kind: "item" as const, id: "docs", label: m.helpDocs, onSelect: () => open(docsUrl) }]),
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
