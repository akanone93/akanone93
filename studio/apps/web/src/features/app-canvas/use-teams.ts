// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useParams } from "@tanstack/react-router";
import { organizationIdOf } from "../../lib/session.ts";
import { useTeamNames } from "../apps/use-apps.ts";

export interface TeamOption {
  readonly id: string;
  readonly name: string;
}

/**
 * The teams of the organisation, for the owning-team picker (D17); the server lists only what the caller may see.
 * The organisation home's query, shared: two queries with one key and different shapes left the cache holding the
 * canvas's shape, and the home's list crashed on it after the way back ("map is not a function").
 */
export function useTeams(): readonly TeamOption[] {
  const organizationId = organizationIdOf((useParams({ strict: false }) as { org?: string }).org ?? "");
  return useTeamNames(organizationId);
}
