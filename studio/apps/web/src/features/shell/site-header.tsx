// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon } from "@plux/ui";
import { Breadcrumb, BreadcrumbItem, BreadcrumbList, BreadcrumbPage } from "@plux/ui/components/breadcrumb";
import { Button } from "@plux/ui/components/button";
import { Kbd } from "@plux/ui/components/kbd";
import { Separator } from "@plux/ui/components/separator";
import { SidebarTrigger } from "@plux/ui/components/sidebar";
import type { ReactElement, ReactNode } from "react";
import { commandMessages, formatShortcut } from "../../lib/commands/index.ts";
import { HelpMenu } from "./help-menu.tsx";
import { NotificationsPopover } from "./notifications-popover.tsx";
import { chromeMessages } from "./shell-chrome.messages.ts";
import { ThemeButton } from "./theme-button.tsx";

/**
 * The top of every signed-in page: shadcn's site header (dashboard-01). The sidebar trigger and where the person is on
 * the start side; status, search, theme, notifications and help on the end side.
 */
export function SiteHeader({
  title,
  status,
  docsUrl,
  onSearch,
}: {
  /** Where the person is: the organisation's name, or nothing while it loads. */
  readonly title: string;
  readonly status: ReactNode;
  readonly docsUrl: string | undefined;
  readonly onSearch: () => void;
}): ReactElement {
  const { i18n } = useLingui();
  const m = chromeMessages(i18n);
  const c = commandMessages(i18n);
  const apple = typeof navigator !== "undefined" && /mac|iphone|ipad/i.test(navigator.platform);
  return (
    <header className="flex h-(--header-height) shrink-0 items-center gap-2 border-b">
      <div className="flex w-full items-center gap-1 px-4 lg:gap-2 lg:px-6">
        <SidebarTrigger className="-ms-1" aria-label={m.toggleSidebar} />
        <Separator orientation="vertical" className="mx-2 h-4 data-vertical:self-auto" />
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbPage className="font-medium">{title}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
        <div className="ms-auto flex items-center gap-2">
          {status}
          <Button
            variant="outline"
            size="sm"
            className="hidden text-muted-foreground sm:inline-flex"
            onClick={onSearch}
          >
            <Icon name="magnifying-glass" data-icon="inline-start" />
            {c.paletteOpen}
            <Kbd>{formatShortcut("mod+k", apple)}</Kbd>
          </Button>
          <ThemeButton />
          <NotificationsPopover />
          <HelpMenu docsUrl={docsUrl} />
        </div>
      </div>
    </header>
  );
}
