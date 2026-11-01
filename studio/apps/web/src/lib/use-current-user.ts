// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useQuery } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { currentUserQuery } from "./guards.ts";
import { organizationIdOf } from "./session.ts";
import { useApp } from "./use-app.tsx";

/** The `:org` of the page as an organisation ID, or undefined outside any organisation. */
export function usePageOrganization(): string | undefined {
  const { org } = useParams({ strict: false }) as { org?: string };
  return org === undefined ? undefined : organizationIdOf(org);
}

/**
 * The signed-in user with their permissions in an organisation: the page's own unless one is named. It shares the
 * cache entry the route guards loaded, so a page sees the same permissions its guard checked.
 */
export function useCurrentUser(organization?: string) {
  const { session } = useApp();
  const page = usePageOrganization();
  return useQuery(currentUserQuery(session, organization ?? page));
}
