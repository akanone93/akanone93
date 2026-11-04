// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";
import { useLingui } from "@lingui/react";
import type { IconName } from "@plux/ui";
import { useRouterState } from "@tanstack/react-router";
import { adminPages } from "../admin/admin-pages.ts";
import { approvalsMessages } from "../approvals/approvals.messages.ts";
import { installationMessages } from "../installation/installation.messages.ts";
import { appNavMessages } from "./app-nav.messages.ts";
import { entryIcons } from "./app-nav.tsx";
import { notifyMessages } from "./notify.messages.ts";
import { chromeMessages } from "./shell-chrome.messages.ts";

/** The area a page belongs to: the sidebar's icon and name for it, and the page's own name where the sidebar lists it. */
export interface PageArea {
  readonly icon: IconName;
  readonly label: string;
  readonly page?: string;
}

/** The page of `pages` that `rest` (the path after the area's root) opens: the longest match, the landing page last. */
function pageOf(pages: readonly { path: string; label: string }[], rest: string): string | undefined {
  const hit = [...pages]
    .filter((p) => p.path !== "" && (rest === p.path || rest.startsWith(`${p.path}/`)))
    .sort((a, b) => b.path.length - a.path.length)[0];
  return hit?.label ?? pages.find((p) => p.path === "" && rest === "")?.label;
}

function orgArea(i18n: I18n, section: string, rest: string): PageArea | undefined {
  const m = chromeMessages(i18n);
  switch (section) {
    case "admin": {
      const page = pageOf(adminPages(i18n), rest);
      return { icon: "gear", label: m.admin, ...(page === undefined ? {} : { page }) };
    }
    case "approvals": {
      const a = approvalsMessages(i18n);
      const pages = [
        { path: "", label: a.navInbox },
        { path: "/history", label: a.navHistory },
        { path: "/policies", label: a.navPolicies },
        { path: "/break-glass", label: a.navBreakGlass },
      ];
      const page = pageOf(pages, rest);
      return { icon: "check-circle", label: m.approvals, ...(page === undefined ? {} : { page }) };
    }
    case "library":
      return { icon: "book", label: m.library };
    case "notifications":
      return { icon: "bell", label: notifyMessages(i18n).bell };
    default:
      return undefined;
  }
}

/**
 * The area of `pathname`, as the sidebar names it: the organisation's pages, the installation's, an app's areas and the
 * profile. Pages outside every area (the home page, sign-in) have none.
 */
export function areaOf(i18n: I18n, pathname: string): PageArea | undefined {
  const parts = pathname.split("/").filter((p) => p !== "");
  if (parts[0] === "installation") {
    const m = installationMessages(i18n);
    const pages = [
      { path: "", label: m.navHealth },
      { path: "/organisations", label: m.navOrganisations },
      { path: "/storage", label: m.navStorage },
      { path: "/jobs", label: m.navJobs },
      { path: "/backups", label: m.navBackups },
      { path: "/issuers", label: m.navIssuers },
      { path: "/audit", label: m.navAudit },
    ];
    const page = pageOf(pages, `/${parts.slice(1).join("/")}`.replace(/\/$/, ""));
    return { icon: "server", label: m.navLabel, ...(page === undefined ? {} : { page }) };
  }
  if (parts[0] === "me") {
    return { icon: "user", label: chromeMessages(i18n).profile };
  }
  if (parts[0] !== "o" || parts.length < 3) {
    return undefined;
  }
  if (parts[2] === "apps" && parts.length >= 5) {
    const id = parts[4] ?? "";
    const names = appNavMessages(i18n) as unknown as Readonly<Record<string, string>>;
    const icon = entryIcons[id];
    return icon === undefined || names[id] === undefined ? undefined : { icon, label: names[id] };
  }
  return orgArea(i18n, parts[2] ?? "", `/${parts.slice(3).join("/")}`.replace(/\/$/, ""));
}

/** The area of the page on screen. */
export function usePageArea(): PageArea | undefined {
  const { i18n } = useLingui();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  return areaOf(i18n, pathname);
}
