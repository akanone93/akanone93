// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";
import { adminMessages } from "./admin.messages.ts";

/**
 * The administration pages in navigation order, each with the permission its route checks (router.tsx, F14). The
 * sidebar lists only those the person may open.
 */
export function adminPages(i18n: I18n) {
  const m = adminMessages(i18n);
  return [
    { path: "", label: m.navSettings, permission: "organization.manage" },
    { path: "/members", label: m.navMembers, permission: "members.manage" },
    { path: "/teams", label: m.navTeams, permission: "teams.manage" },
    { path: "/roles", label: m.navRoles, permission: "roles.manage" },
    { path: "/restrictions", label: m.navRestrictions, permission: "restrictions.manage" },
    { path: "/access", label: m.navAccess, permission: "roles.manage" },
    { path: "/sso", label: m.navSso, permission: "sso.manage" },
    { path: "/tokens", label: m.navTokens, permission: "tokens.manage" },
    { path: "/sessions", label: m.navSessions, permission: "sessions.manage" },
    { path: "/audit", label: m.navAudit, permission: "audit.read" },
    { path: "/compliance", label: m.navCompliance, permission: "compliance.report" },
    { path: "/retention", label: m.navRetention, permission: "retention.manage" },
    { path: "/license", label: m.navLicense, permission: "organization.manage" },
    { path: "/limits", label: m.navLimits, permission: "limits.manage" },
  ] as const;
}
