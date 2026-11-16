// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { useLingui } from "@lingui/react";
import { AppService } from "@plux/api";
import { Icon } from "@plux/ui";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@plux/ui/components/breadcrumb";
import { Button } from "@plux/ui/components/button";
import { Kbd } from "@plux/ui/components/kbd";
import { Separator } from "@plux/ui/components/separator";
import { SidebarTrigger } from "@plux/ui/components/sidebar";
import { useQuery } from "@tanstack/react-query";
import { Link, useLocation } from "@tanstack/react-router";
import { type ReactElement, type ReactNode, useMemo } from "react";
import { commandMessages, formatShortcut } from "../../lib/commands/index.ts";
import { useApp } from "../../lib/use-app.tsx";
import { HelpMenu } from "./help-menu.tsx";
import { NotificationsPopover } from "./notifications-popover.tsx";
import { usePageArea } from "./page-area.ts";
import { chromeMessages } from "./shell-chrome.messages.ts";
import { ThemeButton } from "./theme-button.tsx";

/** The organisation and app of an app's page (`/o/<org>/apps/<app>/…`), or undefined on any other page. */
export function appOfPath(pathname: string): { readonly org: string; readonly app: string } | undefined {
  const m = /^\/o\/([^/]+)\/apps\/([^/]+)/.exec(pathname);
  return m === null || m[2] === "new" ? undefined : { org: m[1] ?? "", app: m[2] ?? "" };
}

/** The app an app's page belongs to, by name, linking to its canvas: nothing else on such a page names it. */
function AppCrumb(): ReactElement | null {
  const { session } = useApp();
  const at = appOfPath(useLocation().pathname);
  const apps = useMemo(() => createClient(AppService, session.transport), [session.transport]);
  const app = useQuery({
    queryKey: ["app", at?.app ?? ""],
    queryFn: async () => (await apps.getApp({ id: at?.app ?? "" })).app,
    enabled: at !== undefined,
  });
  const name = app.data?.name;
  if (at === undefined || name === undefined || name === "") {
    return null;
  }
  return (
    <>
      <BreadcrumbSeparator className="hidden md:block" />
      <BreadcrumbItem className="hidden md:inline-flex">
        <Link
          to="/o/$org/apps/$app"
          params={{ org: at.org, app: at.app }}
          className="max-w-48 truncate text-muted-foreground hover:text-foreground"
        >
          {name}
        </Link>
      </BreadcrumbItem>
    </>
  );
}

/**
 * The top of every signed-in page: shadcn's site header (dashboard-01). The sidebar trigger and where the person is on
 * the start side; status, search, theme, notifications and help on the end side.
 */
export function SiteHeader({
  title,
  status,
  docsUrl,
  onSearch,
  gettingStarted,
  onShowGettingStarted,
}: {
  /** Where the person is: the organisation's name, or nothing while it loads. */
  readonly title: string;
  readonly status: ReactNode;
  readonly docsUrl: string | undefined;
  readonly onSearch: () => void;
  /** The Get started button of an organisation with steps left (STU-015). */
  readonly gettingStarted: ReactNode;
  readonly onShowGettingStarted: (() => void) | undefined;
}): ReactElement {
  const { i18n } = useLingui();
  const m = chromeMessages(i18n);
  const c = commandMessages(i18n);
  const area = usePageArea();
  const apple = typeof navigator !== "undefined" && /mac|iphone|ipad/i.test(navigator.platform);
  return (
    // The header sits inside shadcn's inset, a main landmark, so it names its banner role itself.
    // biome-ignore lint/a11y/noInteractiveElementToNoninteractiveRole: a header inside main is no banner without the role
    <header
      role="banner"
      className="site-header sticky top-0 z-20 flex h-(--header-height) shrink-0 items-center gap-2 rounded-t-xl border-b"
    >
      <div className="flex w-full items-center gap-1 px-4 lg:gap-2 lg:px-6">
        <SidebarTrigger className="-ms-1" aria-label={m.toggleSidebar} />
        <Separator orientation="vertical" className="mx-2 h-4 data-vertical:self-auto" />
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbPage className={area === undefined ? "font-medium" : "text-muted-foreground"}>
                {title}
              </BreadcrumbPage>
            </BreadcrumbItem>
            <AppCrumb />
            {area !== undefined && (
              <>
                <BreadcrumbSeparator />
                <BreadcrumbItem className="hidden md:inline-flex">
                  <Icon name={area.icon} size="pill" />
                  <BreadcrumbPage className={area.page === undefined ? "font-medium" : "text-muted-foreground"}>
                    {area.label}
                  </BreadcrumbPage>
                </BreadcrumbItem>
              </>
            )}
            {area?.page !== undefined && (
              <>
                <BreadcrumbSeparator className="hidden md:block" />
                <BreadcrumbItem className="hidden md:inline-flex">
                  <BreadcrumbPage className="font-medium">{area.page}</BreadcrumbPage>
                </BreadcrumbItem>
              </>
            )}
          </BreadcrumbList>
        </Breadcrumb>
        <div className="ms-auto flex items-center gap-2">
          {status}
          {gettingStarted}
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
          <HelpMenu docsUrl={docsUrl} onShowGettingStarted={onShowGettingStarted} />
        </div>
      </div>
    </header>
  );
}
