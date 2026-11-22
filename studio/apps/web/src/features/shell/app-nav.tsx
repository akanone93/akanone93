// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, type IconName } from "@plux/ui";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@plux/ui/components/dropdown-menu";
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@plux/ui/components/sidebar";
import { Link } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { appNavMessages } from "./app-nav.messages.ts";
import type { NavApp } from "./use-current-app.ts";

/** One area of an app: where it lives and the permission its route guard needs (router.tsx). */
export interface AppNavEntry {
  readonly id: string;
  readonly to: string;
  readonly permission: string;
  /** True when the area is organisation-wide and the app is passed as a search parameter. */
  readonly orgWide: boolean;
}

/** The areas of an app in navigation order; each permission is the one the route's `beforeLoad` requires. */
export const appNavEntries: readonly AppNavEntry[] = [
  { id: "canvas", to: "/o/$org/apps/$app", permission: "app.read", orgWide: false },
  { id: "releases", to: "/o/$org/apps/$app/releases", permission: "app.read", orgWide: false },
  { id: "functions", to: "/o/$org/apps/$app/functions", permission: "app.read", orgWide: false },
  { id: "data", to: "/o/$org/apps/$app/data", permission: "app.read", orgWide: false },
  { id: "localisation", to: "/o/$org/apps/$app/localisation", permission: "app.read", orgWide: false },
  { id: "design", to: "/o/$org/apps/$app/design", permission: "app.read", orgWide: false },
  { id: "analytics", to: "/o/$org/apps/$app/analytics", permission: "app.read", orgWide: false },
  { id: "tests", to: "/o/$org/apps/$app/tests", permission: "app.read", orgWide: false },
  { id: "experiments", to: "/o/$org/apps/$app/experiments", permission: "app.read", orgWide: false },
  { id: "approvals", to: "/o/$org/approvals", permission: "app.read", orgWide: true },
  { id: "publish", to: "/o/$org/apps/$app/publish", permission: "release.publish", orgWide: false },
  { id: "devices", to: "/o/$org/apps/$app/devices", permission: "app.read", orgWide: false },
  { id: "security", to: "/o/$org/apps/$app/security", permission: "app.read", orgWide: false },
  { id: "settings", to: "/o/$org/apps/$app/settings", permission: "app.read", orgWide: false },
];

/** The icon beside each area in the sidebar. */
export const entryIcons: Readonly<Record<string, IconName>> = {
  canvas: "flow-arrow",
  releases: "rocket-launch",
  functions: "lightning",
  data: "stack",
  localisation: "globe",
  design: "pencil-simple",
  analytics: "pulse",
  tests: "flask",
  experiments: "funnel",
  approvals: "check-circle",
  publish: "paper-plane-tilt",
  devices: "device-mobile",
  security: "lock",
  settings: "gear",
};

/** The entries a person with `permissions` may open: the same checks the route guards make. */
export function visibleAppNav(permissions: readonly string[]): readonly AppNavEntry[] {
  return appNavEntries.filter((e) => permissions.includes(e.permission));
}

/** The switcher above the app's areas: the app they are about, and the other apps the person may open. */
function AppSwitcher({
  app,
  apps,
  onPick,
}: {
  readonly app: string;
  readonly apps: readonly NavApp[];
  readonly onPick: (appId: string) => void;
}): ReactElement {
  const m = appNavMessages(useLingui().i18n);
  const name = apps.find((a) => a.id === app)?.name ?? m.thisApp;
  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger render={<SidebarMenuButton aria-label={m.switchApp(name)} className="font-medium" />}>
        <Icon name="squares-four" />
        <span className="truncate">{name}</span>
        <Icon name="expand-all" className="ms-auto" />
      </DropdownMenuTrigger>
      <DropdownMenuContent className="min-w-56" align="start" sideOffset={4}>
        <DropdownMenuGroup>
          <DropdownMenuLabel className="text-xs text-muted-foreground">{m.apps}</DropdownMenuLabel>
          {apps.map((a) => (
            <DropdownMenuItem key={a.id} onClick={() => onPick(a.id)} className="gap-2">
              <span className="truncate">{a.name}</span>
              {a.id === app && <Icon name="check" className="ms-auto" />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/**
 * The areas of the current app (#202), as a group of the sidebar: the app the one rule of `chooseCurrentApp` names,
 * with a switcher, so the group stays on every page of the organisation instead of only on pages whose address holds
 * the app. It renders only entries the person's permissions allow, by the same checks the route guards make.
 */
export function AppNav({
  org,
  app,
  apps,
  permissions,
  onPickApp,
}: {
  readonly org: string;
  readonly app: string;
  readonly apps: readonly NavApp[];
  readonly permissions: readonly string[];
  readonly onPickApp: (appId: string) => void;
}): ReactElement | null {
  const { i18n } = useLingui();
  const m = appNavMessages(i18n);
  const entries = visibleAppNav(permissions);
  if (entries.length === 0) {
    return null;
  }
  const names = m as unknown as Readonly<Record<string, string>>;
  return (
    <nav aria-label={m.label}>
      <SidebarGroup>
        <SidebarGroupLabel>{m.label}</SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu>
            <SidebarMenuItem>
              <AppSwitcher app={app} apps={apps} onPick={onPickApp} />
            </SidebarMenuItem>
            {entries.map((e) => (
              <SidebarMenuItem key={e.id}>
                <SidebarMenuButton
                  render={
                    <Link
                      to={e.to}
                      params={{ org, app }}
                      {...(e.orgWide && { search: { app } })}
                      activeOptions={{ exact: e.id === "canvas" || e.orgWide, includeSearch: false }}
                      activeProps={{ "aria-current": "page", "data-active": true }}
                    />
                  }
                >
                  <Icon name={entryIcons[e.id] ?? "file"} />
                  <span>{names[e.id]}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            ))}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </nav>
  );
}
