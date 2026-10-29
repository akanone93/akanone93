// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { z } from "zod";

/** The ways the apps can be ordered (STU-023). */
export const appSorts = ["name", "activity", "health"] as const;
export type AppSort = (typeof appSorts)[number];

/** Which apps the status filter keeps (STU-023). */
export const appStatuses = ["active", "archived", "all"] as const;
export type AppStatus = (typeof appStatuses)[number];

/** O2 search: `?view&q&team&tag&status&mine&sort`. Bad values fall back to the defaults. */
export const appsSearchSchema = z.object({
  view: z.enum(["grid", "list"]).optional().catch(undefined),
  q: z.string().optional().catch(undefined),
  team: z.string().optional().catch(undefined),
  tag: z.string().optional().catch(undefined),
  status: z.enum(appStatuses).optional().catch(undefined),
  mine: z.boolean().optional().catch(undefined),
  sort: z.enum(appSorts).optional().catch(undefined),
});

export type AppsSearch = z.infer<typeof appsSearchSchema>;
