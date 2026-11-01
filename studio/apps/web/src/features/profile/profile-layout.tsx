// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, type IconName } from "@plux/ui";
import { buttonVariants } from "@plux/ui/components/button";
import { Separator } from "@plux/ui/components/separator";
import { Link } from "@tanstack/react-router";
import type { ReactElement, ReactNode } from "react";
import { PageHeader } from "../../components/page.tsx";
import { profileMessages } from "./profile.messages.ts";

const tabs: readonly {
  readonly to: "/me" | "/me/shortcuts";
  readonly icon: IconName;
  readonly key: "navProfile" | "navShortcuts";
}[] = [
  { to: "/me", icon: "user", key: "navProfile" },
  { to: "/me/shortcuts", icon: "keyboard", key: "navShortcuts" },
];

/**
 * The person's own pages (U2, U3) as one place with two tabs: shadcn's settings layout, the tabs beside the content on
 * a wide screen and above it on a narrow one.
 */
export function ProfileLayout({
  id,
  title,
  caption,
  actions,
  children,
}: {
  readonly id: string;
  readonly title: string;
  readonly caption: string;
  readonly actions?: ReactNode;
  readonly children: ReactNode;
}): ReactElement {
  const m = profileMessages(useLingui().i18n);
  return (
    <section aria-labelledby={id} className="flex flex-col gap-6">
      <PageHeader id={id} title={title} caption={caption} actions={actions} />
      <Separator />
      <div className="flex flex-col gap-6 lg:flex-row lg:gap-12">
        <nav aria-label={m.navLabel} className="flex gap-1 lg:w-48 lg:shrink-0 lg:flex-col">
          {tabs.map((tab) => (
            <Link
              key={tab.to}
              to={tab.to}
              activeOptions={{ exact: true }}
              activeProps={{ className: "bg-muted" }}
              className={buttonVariants({ variant: "ghost", className: "justify-start" })}
            >
              <Icon name={tab.icon} data-icon="inline-start" />
              {m[tab.key]}
            </Link>
          ))}
        </nav>
        <div className="flex min-w-0 flex-1 flex-col gap-6 lg:max-w-3xl">{children}</div>
      </div>
    </section>
  );
}
