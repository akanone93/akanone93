// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon } from "@plux/ui";
import { Button } from "@plux/ui/components/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@plux/ui/components/dropdown-menu";
import { Link } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { organizationIdOf } from "../../lib/session.ts";
import { useCurrentUser } from "../../lib/use-current-user.ts";
import { appNavMessages } from "./app-nav.messages.ts";
import { entryIcons, visibleAppNav } from "./app-nav.tsx";

/**
 * The app's pages from inside a canvas (#200, #202): the canvases are full-screen and have no sidebar, so this menu
 * lists the same entries as the sidebar's app areas, by the same permission checks (`visibleAppNav`).
 */
export function AppPagesMenu({ org, app }: { readonly org: string; readonly app: string }): ReactElement | null {
  const m = appNavMessages(useLingui().i18n);
  const user = useCurrentUser(organizationIdOf(org));
  const entries = visibleAppNav(user.data?.permissions ?? []).filter((e) => e.id !== "canvas");
  if (entries.length === 0 || app === "") {
    return null;
  }
  const names = m as unknown as Readonly<Record<string, string>>;
  return (
    <DropdownMenu modal={false}>
      {/* A visible label, not an icon behind a tooltip: the way to the app's pages has to be found at a glance. */}
      <DropdownMenuTrigger render={<Button variant="ghost" size="sm" className="shrink-0" />}>
        <Icon name="squares-four" data-icon="inline-start" />
        {m.appPages}
      </DropdownMenuTrigger>
      <DropdownMenuContent className="min-w-52" align="start" sideOffset={6}>
        <DropdownMenuLabel className="text-xs text-muted-foreground">{m.appPages}</DropdownMenuLabel>
        {entries.map((e) => (
          <DropdownMenuItem
            key={e.id}
            render={<Link to={e.to} params={{ org, app }} {...(e.orgWide && { search: { app } })} />}
          >
            <Icon name={entryIcons[e.id] ?? "file"} />
            {names[e.id]}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
