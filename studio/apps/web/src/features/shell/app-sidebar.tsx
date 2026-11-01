// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon } from "@plux/ui";
import { Avatar, AvatarFallback } from "@plux/ui/components/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@plux/ui/components/dropdown-menu";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@plux/ui/components/sidebar";
import { Link, useRouterState } from "@tanstack/react-router";
import type { ComponentProps, ReactElement } from "react";
import { shellMessages } from "../../shell.messages.ts";
import { adminPages } from "../admin/admin-pages.ts";
import { approvalsMessages } from "../approvals/approvals.messages.ts";
import { installationMessages } from "../installation/installation.messages.ts";
import { AppNav } from "./app-nav.tsx";
import { NavArea } from "./nav-area.tsx";
import { chromeMessages } from "./shell-chrome.messages.ts";

/** The administration area's permission: the one its first page (settings) needs (STU-116). */
export const ADMIN_PERMISSION = "organization.manage";

/** The approvals inbox's permission, as its route guard checks it. */
const APPROVALS_PERMISSION = "app.read";

/** Up to two initials of a name, for an avatar without a picture. */
export function initialsOf(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  const letters = words.length > 1 ? [words[0], words[words.length - 1]] : [words[0] ?? ""];
  return letters
    .map((w) => Array.from(w ?? "")[0] ?? "")
    .join("")
    .toLocaleUpperCase();
}

export interface StudioSidebarProps extends ComponentProps<typeof Sidebar> {
  readonly organisations: readonly { readonly id: string; readonly name: string }[];
  readonly currentId: string | undefined;
  /** The `:org` and `:app` URL parameters of the page, when it has them. */
  readonly org: string | undefined;
  readonly app: string | undefined;
  readonly permissions: readonly string[];
  readonly user: { readonly name: string; readonly email: string } | undefined;
  readonly onSwitch: (organisationId: string) => void;
  /** Opens the create-organisation dialog; undefined unless the person is an installation administrator. */
  readonly onCreateOrganisation: (() => void) | undefined;
  readonly onSignOut: () => void;
  /** Installation administrators also see the installation's pages (STU-116). */
  readonly installationAdmin: boolean;
}

/** The organisation switcher at the top of the sidebar: shadcn's team switcher (sidebar-07). */
function OrganisationSwitcher({
  organisations,
  currentId,
  onSwitch,
  onCreateOrganisation,
}: Pick<StudioSidebarProps, "organisations" | "currentId" | "onSwitch" | "onCreateOrganisation">) {
  const { i18n } = useLingui();
  const m = chromeMessages(i18n);
  const { isMobile } = useSidebar();
  const current = organisations.find((o) => o.id === currentId);
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger
            render={
              <SidebarMenuButton
                size="lg"
                aria-label={m.organisation}
                className="data-open:bg-sidebar-accent data-open:text-sidebar-accent-foreground"
              />
            }
          >
            <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
              <Icon name="stack" />
            </div>
            <div className="grid flex-1 text-start text-sm leading-tight">
              <span className="truncate font-medium">{current?.name ?? m.organisation}</span>
              <span className="truncate text-xs">{shellMessages(i18n).appTitle}</span>
            </div>
            <Icon name="caret-down" className="ms-auto" />
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="min-w-56"
            align="start"
            side={isMobile ? "bottom" : "inline-end"}
            sideOffset={4}
          >
            <DropdownMenuGroup>
              <DropdownMenuLabel className="text-xs text-muted-foreground">{m.organisations}</DropdownMenuLabel>
              {organisations.map((o) => (
                <DropdownMenuItem key={o.id} onClick={() => onSwitch(o.id)} className="gap-2 p-2">
                  <div className="flex size-6 items-center justify-center rounded-md border">
                    <Icon name="stack" size="pill" />
                  </div>
                  {o.name}
                  {o.id === currentId && <Icon name="check" className="ms-auto" />}
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
            {onCreateOrganisation !== undefined && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuGroup>
                  <DropdownMenuItem className="gap-2 p-2" onClick={onCreateOrganisation}>
                    <div className="flex size-6 items-center justify-center rounded-md border bg-transparent">
                      <Icon name="plus" />
                    </div>
                    <div className="font-medium text-muted-foreground">{m.newOrganisation}</div>
                  </DropdownMenuItem>
                </DropdownMenuGroup>
              </>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}

/** The person at the foot of the sidebar with the account menu: shadcn's nav-user (sidebar-07). */
function NavUser({
  user,
  onSignOut,
}: {
  readonly user: { readonly name: string; readonly email: string } | undefined;
  readonly onSignOut: () => void;
}) {
  const m = chromeMessages(useLingui().i18n);
  const { isMobile } = useSidebar();
  const name = user?.name || user?.email || "";
  const avatar = (
    <Avatar className="size-8 rounded-lg">
      <AvatarFallback className="rounded-lg">{initialsOf(name)}</AvatarFallback>
    </Avatar>
  );
  const who = (
    <div className="grid flex-1 text-start text-sm leading-tight">
      <span className="truncate font-medium">{name}</span>
      <span className="truncate text-xs text-muted-foreground">{user?.email}</span>
    </div>
  );
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger
            render={<SidebarMenuButton size="lg" aria-label={m.profile} className="aria-expanded:bg-muted" />}
          >
            {avatar}
            {who}
            <Icon name="caret-down" className="ms-auto" />
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="min-w-56"
            side={isMobile ? "bottom" : "inline-end"}
            align="end"
            sideOffset={4}
          >
            <DropdownMenuGroup>
              <DropdownMenuLabel className="p-0 font-normal">
                <div className="flex items-center gap-2 px-1 py-1.5 text-start text-sm">
                  {avatar}
                  {who}
                </div>
              </DropdownMenuLabel>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem render={<Link to="/me" />}>
                <Icon name="user" />
                {m.profile}
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem onClick={onSignOut}>
                <Icon name="sign-out" />
                {m.signOut}
              </DropdownMenuItem>
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}

/**
 * Studio's sidebar (U9.2): shadcn's sidebar with the organisation switcher on top, the organisation's pages and, inside
 * an app, the app's areas, and the person's account menu at the foot. It shows only links the person may open.
 */
export function StudioSidebar({
  organisations,
  currentId,
  org,
  app,
  permissions,
  user,
  onSwitch,
  onCreateOrganisation,
  onSignOut,
  installationAdmin,
  ...props
}: StudioSidebarProps): ReactElement {
  const { i18n } = useLingui();
  const m = chromeMessages(i18n);
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const home = org === undefined ? undefined : `/o/${org}`;
  const approvals = approvalsMessages(i18n);
  const approvalPages = [
    { path: "", label: approvals.navInbox, permission: APPROVALS_PERMISSION },
    { path: "/history", label: approvals.navHistory, permission: APPROVALS_PERMISSION },
    { path: "/policies", label: approvals.navPolicies, permission: APPROVALS_PERMISSION },
    { path: "/break-glass", label: approvals.navBreakGlass, permission: "audit.read" },
  ];
  const installation = installationMessages(i18n);
  const installationPages = [
    { path: "", label: installation.navHealth },
    { path: "/organisations", label: installation.navOrganisations },
    { path: "/storage", label: installation.navStorage },
    { path: "/jobs", label: installation.navJobs },
    { path: "/backups", label: installation.navBackups },
    { path: "/issuers", label: installation.navIssuers },
    { path: "/audit", label: installation.navAudit },
  ];
  return (
    <Sidebar collapsible="offcanvas" {...props}>
      <SidebarHeader>
        <OrganisationSwitcher
          organisations={organisations}
          currentId={currentId}
          onSwitch={onSwitch}
          onCreateOrganisation={onCreateOrganisation}
        />
      </SidebarHeader>
      <SidebarContent>
        <nav aria-label={m.topBar}>
          <SidebarGroup>
            <SidebarGroupLabel>{m.workspace}</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {org !== undefined && (
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      tooltip={m.home}
                      isActive={pathname === home}
                      render={<Link to="/o/$org" params={{ org }} />}
                    >
                      <Icon name="house" />
                      <span>{m.home}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                )}
                {home !== undefined && (
                  <NavArea
                    icon="check-circle"
                    label={m.approvals}
                    pages={approvalPages
                      .filter((p) => permissions.includes(p.permission))
                      .map((p) => ({ href: `${home}/approvals${p.path}`, label: p.label }))}
                  />
                )}
                {org !== undefined && (
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      tooltip={m.library}
                      isActive={pathname.startsWith(`${home}/library`)}
                      render={<Link to="/o/$org/library" params={{ org }} />}
                    >
                      <Icon name="book" />
                      <span>{m.library}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                )}
                {home !== undefined && (
                  <NavArea
                    icon="gear"
                    label={m.admin}
                    pages={adminPages(i18n)
                      .filter((p) => permissions.includes(p.permission))
                      .map((p) => ({ href: `${home}/admin${p.path}`, label: p.label }))}
                  />
                )}
                {installationAdmin && (
                  <NavArea
                    icon="server"
                    label={installation.navLabel}
                    pages={installationPages.map((p) => ({ href: `/installation${p.path}`, label: p.label }))}
                  />
                )}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </nav>
        {org !== undefined && app !== undefined && <AppNav org={org} app={app} permissions={permissions} />}
      </SidebarContent>
      <SidebarFooter>
        <NavUser user={user} onSignOut={onSignOut} />
      </SidebarFooter>
    </Sidebar>
  );
}
