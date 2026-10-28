// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Link } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { appNavMessages } from "./app-nav.messages.ts";

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

/** The entries a person with `permissions` may open: the same checks the route guards make. */
export function visibleAppNav(permissions: readonly string[]): readonly AppNavEntry[] {
  return appNavEntries.filter((e) => permissions.includes(e.permission));
}

/** Navigation between the areas of the app in the URL; it renders only entries the person's permissions allow. */
export function AppNav({
  org,
  app,
  permissions,
}: {
  readonly org: string;
  readonly app: string;
  readonly permissions: readonly string[];
}): ReactElement | null {
  const { i18n } = useLingui();
  const m = appNavMessages(i18n);
  const entries = visibleAppNav(permissions);
  if (entries.length === 0) {
    return null;
  }
  const names = m as unknown as Readonly<Record<string, string>>;
  return (
    <nav aria-label={m.label} className="mx-4 flex flex-wrap gap-4">
      {entries.map((e) => (
        <Link
          key={e.id}
          to={e.to}
          params={{ org, app }}
          {...(e.id === "approvals" && { search: { app } })}
          className="text-body text-secondary"
          activeProps={{ "aria-current": "page", className: "text-body text-primary" }}
        >
          {names[e.id]}
        </Link>
      ))}
    </nav>
  );
}
