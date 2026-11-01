// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Icon, type IconName } from "@plux/ui";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@plux/ui/components/collapsible";
import {
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from "@plux/ui/components/sidebar";
import { Link, useRouterState } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";

/** One page of an area: its full path and its name. */
export interface AreaPage {
  readonly href: string;
  readonly label: string;
}

/** Whether `pathname` is `href` itself; the area's first page is matched exactly, so it is not current everywhere. */
function isCurrent(pathname: string, href: string, exact: boolean): boolean {
  return exact ? pathname === href : pathname === href || pathname.startsWith(`${href}/`);
}

/**
 * An area of the sidebar with its pages beneath it: shadcn's collapsible nav-main item (sidebar-07). It opens by itself
 * when the person is on one of its pages.
 */
export function NavArea({
  icon,
  label,
  pages,
}: {
  readonly icon: IconName;
  readonly label: string;
  /** The area's pages; the first is its landing page. */
  readonly pages: readonly AreaPage[];
}): ReactElement | null {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const landing = pages[0]?.href;
  const inArea = landing !== undefined && isCurrent(pathname, landing, false);
  const [open, setOpen] = useState(inArea);
  const [wasInArea, setWasInArea] = useState(inArea);
  if (inArea !== wasInArea) {
    setWasInArea(inArea);
    if (inArea) {
      setOpen(true);
    }
  }
  if (pages.length === 0) {
    return null;
  }
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="group/collapsible" render={<SidebarMenuItem />}>
      <CollapsibleTrigger render={<SidebarMenuButton tooltip={label} isActive={inArea && !open} />}>
        <Icon name={icon} />
        <span>{label}</span>
        <Icon
          name="caret-right"
          className="ms-auto transition-transform duration-200 group-data-open/collapsible:rotate-90"
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <SidebarMenuSub>
          {pages.map((page, i) => (
            <SidebarMenuSubItem key={page.href}>
              <SidebarMenuSubButton isActive={isCurrent(pathname, page.href, i === 0)} render={<Link to={page.href} />}>
                <span>{page.label}</span>
              </SidebarMenuSubButton>
            </SidebarMenuSubItem>
          ))}
        </SidebarMenuSub>
      </CollapsibleContent>
    </Collapsible>
  );
}
